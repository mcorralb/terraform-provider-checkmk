package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeOrderServer simulates the CheckMK ruleset collection plus the move
// action. It keeps an in-memory ordered list of rule ids per ruleset and
// applies moves so the reorder logic can be exercised end to end.
type fakeOrderServer struct {
	order map[string][]string // ruleset -> ordered rule ids
}

func (f *fakeOrderServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case r.Method == "GET" && strings.HasSuffix(p, "/rule/collections/all"):
			rs := r.URL.Query().Get("ruleset_name")
			ids := f.order[rs]
			value := make([]map[string]interface{}, 0, len(ids))
			for i, id := range ids {
				value = append(value, map[string]interface{}{
					"id": id,
					"extensions": map[string]interface{}{
						"ruleset":      rs,
						"folder":       "/",
						"folder_index": i,
						"properties": map[string]interface{}{
							"description": "rule " + id,
							"disabled":    false,
						},
						"value_raw":  "{}",
						"conditions": map[string]interface{}{},
					},
				})
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"value": value})
		case r.Method == "GET" && strings.Contains(p, "/objects/rule/"):
			// GET rule (used for ETag lookup)
			id := strings.TrimPrefix(strings.TrimSuffix(p, ""), "/objects/rule/")
			id = strings.TrimPrefix(id, "/")
			w.Header().Set("ETag", `"`+id+`"`)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":         id,
				"extensions": map[string]interface{}{"folder": "/"},
			})
		case r.Method == "POST" && strings.Contains(p, "/actions/move/invoke"):
			// Move action
			id := ""
			// extract rule id from path: /objects/rule/<id>/actions/move/invoke
			parts := strings.Split(p, "/")
			for i, seg := range parts {
				if seg == "rule" && i+1 < len(parts) {
					id = parts[i+1]
					break
				}
			}
			var body RuleMoveRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			// find the ruleset that contains this rule
			rs := ""
			for name, ids := range f.order {
				for _, rid := range ids {
					if rid == id {
						rs = name
						break
					}
				}
			}
			if rs == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			ids := f.order[rs]
			// remove id
			idx := -1
			for i, rid := range ids {
				if rid == id {
					idx = i
					break
				}
			}
			ids = append(ids[:idx], ids[idx+1:]...)
			newIdx := len(ids)
			switch body.Position {
			case MovePositionTopOfFolder:
				newIdx = 0
			case MovePositionBottomOfFolder:
				newIdx = len(ids)
			case MovePositionAfterRule:
				for i, rid := range ids {
					if rid == body.RuleID {
						newIdx = i + 1
						break
					}
				}
			case MovePositionBeforeRule:
				for i, rid := range ids {
					if rid == body.RuleID {
						newIdx = i
						break
					}
				}
			}
			ids = append(ids[:newIdx], append([]string{id}, ids[newIdx:]...)...)
			f.order[rs] = ids
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": id,
				"extensions": map[string]interface{}{
					"folder":       "/",
					"folder_index": newIdx,
					"properties":   map[string]interface{}{"description": "", "disabled": false},
					"value_raw":    "{}",
					"conditions":   map[string]interface{}{},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func TestReorderRules(t *testing.T) {
	tests := []struct {
		name    string
		initial []string
		desired []string
	}{
		{"already ordered", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"fully reversed", []string{"a", "b", "c"}, []string{"c", "b", "a"}},
		{"move first to last", []string{"a", "b", "c"}, []string{"b", "c", "a"}},
		{"move last to first", []string{"a", "b", "c"}, []string{"c", "a", "b"}},
		{"partial shuffle", []string{"a", "b", "c", "d"}, []string{"a", "c", "b", "d"}},
		{"two rules", []string{"x", "y"}, []string{"y", "x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &fakeOrderServer{order: map[string][]string{"rs": append([]string{}, tt.initial...)}}
			server := httptest.NewServer(fs.handler())
			defer server.Close()

			client := &Client{HTTPClient: server.Client(), BaseURL: mustParseURL(server.URL)}
			moves, err := client.ReorderRules(context.Background(), "rs", "/", tt.desired)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := fs.order["rs"]
			if len(got) != len(tt.desired) {
				t.Fatalf("order length %d, want %d", len(got), len(tt.desired))
			}
			for i := range tt.desired {
				if got[i] != tt.desired[i] {
					t.Fatalf("order after reorder = %v, want %v (moves=%d)", got, tt.desired, moves)
				}
			}
		})
	}
}

func TestReorderRulesMissingRule(t *testing.T) {
	fs := &fakeOrderServer{order: map[string][]string{"rs": {"a", "b"}}}
	server := httptest.NewServer(fs.handler())
	defer server.Close()

	client := &Client{HTTPClient: server.Client(), BaseURL: mustParseURL(server.URL)}
	_, err := client.ReorderRules(context.Background(), "rs", "/", []string{"a", "zzz"})
	if err == nil {
		t.Fatal("expected error for missing rule")
	}
	if !contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}
