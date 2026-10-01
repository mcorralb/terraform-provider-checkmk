package client

import (
	"context"
	"fmt"
)

// GetRuleOrder returns the ids of the given rules (identified by their CheckMK
// UUID) in the order they appear within the ruleset+folder, as reported by the
// ruleset collection. Rules that do not exist in the ruleset or that live in a
// different folder are reported as errors so callers can distinguish "rule
// removed" / "rule moved elsewhere" from an out-of-order sequence.
func (c *Client) GetRuleOrder(ctx context.Context, ruleset, folder string, ids []string) ([]string, error) {
	rules, err := c.GetRulesByRuleset(ctx, ruleset)
	if err != nil {
		return nil, err
	}

	// Build the real order of the wanted ids within the requested folder.
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}

	order := make([]string, 0, len(ids))
	for _, r := range rules {
		if !wanted[r.ID] {
			continue
		}
		if r.Extensions.Folder != folder {
			return nil, fmt.Errorf("rule %s is in folder %q, expected %q", r.ID, r.Extensions.Folder, folder)
		}
		order = append(order, r.ID)
	}

	// Every wanted id must be present exactly once.
	for _, id := range ids {
		if !wanted[id] {
			return nil, fmt.Errorf("duplicate rule id %s in desired order", id)
		}
		found := false
		for _, oid := range order {
			if oid == id {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("rule %s not found in ruleset %q folder %q", id, ruleset, folder)
		}
	}

	return order, nil
}

// ReorderRules brings the given rules into the exact desired order within the
// ruleset+folder, using the CheckMK move action sequentially. Rules not listed
// in desired (unmanaged rules) are left alone as far as possible: the listed
// rules are moved relative to each other so that their subsequence matches
// desired exactly. Every move fetches a fresh ETag first (the move action
// requires If-Match). Returns the number of moves executed.
func (c *Client) ReorderRules(ctx context.Context, ruleset, folder string, desired []string) (int, error) {
	if len(desired) == 0 {
		return 0, nil
	}

	current, err := c.GetRuleOrder(ctx, ruleset, folder, desired)
	if err != nil {
		return 0, err
	}

	moves := 0
	for i := 0; i < len(desired); i++ {
		if current[i] == desired[i] {
			continue
		}

		// Locate desired[i] in the current order.
		j := -1
		for k, id := range current {
			if id == desired[i] {
				j = k
				break
			}
		}
		if j == -1 {
			return moves, fmt.Errorf("rule %s disappeared from ruleset %q folder %q while reordering", desired[i], ruleset, folder)
		}

		var req RuleMoveRequest
		if i == 0 {
			req = RuleMoveRequest{Position: MovePositionTopOfFolder, Folder: folder}
		} else {
			req = RuleMoveRequest{Position: MovePositionAfterRule, RuleID: desired[i-1]}
		}

		etagResp, err := c.GetRuleWithETag(ctx, desired[i])
		if err != nil {
			return moves, err
		}
		if _, err := c.MoveRule(ctx, desired[i], &req, etagResp.ETag); err != nil {
			return moves, err
		}

		// Update the local view: remove from j, insert at i.
		current = append(current[:j], current[j+1:]...)
		current = append(current[:i], append([]string{desired[i]}, current[i:]...)...)
		moves++
	}

	return moves, nil
}
