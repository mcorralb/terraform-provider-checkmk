package rules

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/terraform-provider-checkmk/internal/common"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource = &RulesetOrderResource{}
)

func NewRulesetOrderResource() resource.Resource {
	return &RulesetOrderResource{}
}

// RulesetOrderResource manages the order of the managed rules within a single
// (ruleset, folder) pair. CheckMK evaluates first-match rulesets top-down, so
// the position of a rule determines which rule wins when several match the
// same host+service. This resource declares the exact desired order of the
// rules (by their CheckMK api_id) and detects/corrects any reordering done
// outside Terraform.
type RulesetOrderResource struct {
	providerData *common.ProviderData
}

// RulesetOrderResourceModel describes the resource data model.
type RulesetOrderResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Ruleset types.String `tfsdk:"ruleset"`
	Folder  types.String `tfsdk:"folder"`
	Rules   types.List   `tfsdk:"rules"`
}

func (r *RulesetOrderResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ruleset_order"
}

func (r *RulesetOrderResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Declares the exact order of the rules within a (ruleset, folder) pair. " +
			"`rules` is the list of CheckMK api_ids in the desired order. Any reordering done outside " +
			"Terraform is detected on the next plan and corrected on apply via the CheckMK move action. " +
			"Rules not listed here are ignored (their position is not managed). Deleting this resource " +
			"does NOT reorder or delete the rules; it only stops managing the order.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Identifier for the order resource (format `<ruleset>@<folder>`).",
				Computed:            true,
			},
			"ruleset": schema.StringAttribute{
				MarkdownDescription: "The ruleset name whose rule order is managed (e.g., 'checkgroup_parameters:filesystem').",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"folder": schema.StringAttribute{
				MarkdownDescription: "The folder within the ruleset whose rule order is managed (e.g., '/').",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"rules": schema.ListAttribute{
				MarkdownDescription: "List of CheckMK api_ids of the managed rules, in the exact order they must have " +
					"within the ruleset+folder. The actual order is read on refresh, so a reorder done outside " +
					"Terraform shows as a diff against this list.",
				Required:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *RulesetOrderResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.providerData = common.ConfigureResource(req, resp)
}

func rulesetOrderID(ruleset, folder string) string {
	return ruleset + "@" + folder
}

func rulesFromList(ctx context.Context, l types.List) ([]string, error) {
	if l.IsNull() || l.IsUnknown() {
		return nil, nil
	}
	var ids []string
	if diags := l.ElementsAs(ctx, &ids, false); diags.HasError() {
		return nil, fmt.Errorf("unable to read rules list: %v", diags.Errors())
	}
	return ids, nil
}

func rulesToList(ctx context.Context, ids []string) (types.List, error) {
	if ids == nil {
		ids = []string{}
	}
	l, diags := types.ListValueFrom(ctx, types.StringType, ids)
	if diags.HasError() {
		return types.ListNull(types.StringType), fmt.Errorf("unable to build rules list: %v", diags.Errors())
	}
	return l, nil
}

func (r *RulesetOrderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RulesetOrderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	desired, err := rulesFromList(ctx, data.Rules)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}
	if len(desired) == 0 {
		resp.Diagnostics.AddError("Validation Error", "rules must list at least one rule api_id on creation")
		return
	}

	cfg := common.BuildSimpleBaseConfig(r.providerData, types.BoolNull())

	moves, err := r.providerData.Client.ReorderRules(ctx, data.Ruleset.ValueString(), data.Folder.ValueString(), desired)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to order rules: %s", err))
		return
	}

	data.ID = types.StringValue(rulesetOrderID(data.Ruleset.ValueString(), data.Folder.ValueString()))

	rulesList, lerr := rulesToList(ctx, desired)
	if lerr != nil {
		resp.Diagnostics.AddError("Client Error", lerr.Error())
		return
	}
	data.Rules = rulesList

	if moves > 0 {
		if err := common.TrackAndActivate(ctx, r.providerData, cfg, "ruleset_order"); err != nil {
			common.AddActivationWarning(resp, "Ruleset order", "created", err)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RulesetOrderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RulesetOrderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	desired, err := rulesFromList(ctx, data.Rules)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}
	if len(desired) == 0 {
		resp.Diagnostics.AddError("Client Error", "rules list is empty in state; run import or re-apply")
		return
	}

	real, err := r.providerData.Client.GetRuleOrder(ctx, data.Ruleset.ValueString(), data.Folder.ValueString(), desired)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read ruleset order: %s", err))
		return
	}

	data.ID = types.StringValue(rulesetOrderID(data.Ruleset.ValueString(), data.Folder.ValueString()))

	rulesList, lerr := rulesToList(ctx, real)
	if lerr != nil {
		resp.Diagnostics.AddError("Client Error", lerr.Error())
		return
	}
	data.Rules = rulesList

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RulesetOrderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RulesetOrderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	desired, err := rulesFromList(ctx, data.Rules)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", err.Error())
		return
	}
	if len(desired) == 0 {
		resp.Diagnostics.AddError("Validation Error", "rules must list at least one rule api_id")
		return
	}

	cfg := common.BuildSimpleBaseConfig(r.providerData, types.BoolNull())

	moves, err := r.providerData.Client.ReorderRules(ctx, data.Ruleset.ValueString(), data.Folder.ValueString(), desired)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to order rules: %s", err))
		return
	}

	data.ID = types.StringValue(rulesetOrderID(data.Ruleset.ValueString(), data.Folder.ValueString()))

	rulesList, lerr := rulesToList(ctx, desired)
	if lerr != nil {
		resp.Diagnostics.AddError("Client Error", lerr.Error())
		return
	}
	data.Rules = rulesList

	if moves > 0 {
		if err := common.TrackAndActivate(ctx, r.providerData, cfg, "ruleset_order"); err != nil {
			common.AddActivationWarning(resp, "Ruleset order", "updated", err)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete is a no-op on CheckMK: the resource only manages the desired order of
// rules that are owned by other checkmk_rule resources. Removing it from the
// configuration must not reorder or delete anything.
func (r *RulesetOrderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}
