# Order example: manage the precedence of filesystem rules
# CheckMK evaluates first-match rulesets top-down, so within the same
# (ruleset, folder) the first rule that matches a host+service wins.
# checkmk_ruleset_order declares the exact desired order (by api_id) and
# detects/corrects any reordering done outside Terraform.

# Two filesystem rules for the same filesystem "/" on the same host: the
# first one in the folder wins.

resource "checkmk_rule" "fs_high" {
  ruleset   = "checkgroup_parameters:filesystem"
  folder    = "/"
  value_raw = "{'levels': (80.0, 90.0)}"

  properties = {
    description = "Tight filesystem levels for prod"
    comment     = "Managed by Terraform"
    disabled    = false
  }

  conditions = {
    host_name = {
      match_on = ["srv-prod-01"]
      operator = "one_of"
    }
    service_description = {
      match_on = ["/"]
      operator = "one_of"
    }
  }
}

resource "checkmk_rule" "fs_default" {
  ruleset   = "checkgroup_parameters:filesystem"
  folder    = "/"
  value_raw = "{'levels': (95.0, 98.0)}"

  properties = {
    description = "Default filesystem levels"
    comment     = "Managed by Terraform"
    disabled    = false
  }

  conditions = {
    service_description = {
      match_on = ["/"]
      operator = "one_of"
    }
  }
}

# The tight rule must win, so it must be declared before the default one.
# rules references the api_id (computed) of the managed rules; never hardcode
# raw UUIDs here, so replacements are tracked automatically.
resource "checkmk_ruleset_order" "fs_root" {
  ruleset = "checkgroup_parameters:filesystem"
  folder  = "/"
  rules = [
    checkmk_rule.fs_high.api_id,
    checkmk_rule.fs_default.api_id,
  ]
}