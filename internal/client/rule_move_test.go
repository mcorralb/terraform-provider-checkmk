package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMoveRule(t *testing.T) {
	tests := []struct {
		name           string
		ruleID         string
		request        *RuleMoveRequest
		etag           string
		responseStatus int
		responseBody   string
		expectError    bool
		errorContains  string
		expectFolder   string
		expectIndex    int
	}{
		{
			name:   "move to top of folder",
			ruleID: "rule-1",
			request: &RuleMoveRequest{
				Position: MovePositionTopOfFolder,
				Folder:   "/",
			},
			etag:           `"abc123"`,
			responseStatus: http.StatusOK,
			responseBody: `{
				"id": "rule-1",
				"title": "Test rule",
				"domainType": "rule",
				"extensions": {
					"ruleset": "host_label_rules",
					"folder": "/",
					"folder_index": 0,
					"properties": {"description": "Test rule", "disabled": false},
					"value_raw": "{'os': 'linux'}",
					"conditions": {}
				}
			}`,
			expectFolder: "/",
			expectIndex:  0,
		},
		{
			name:   "move before specific rule",
			ruleID: "rule-2",
			request: &RuleMoveRequest{
				Position: MovePositionBeforeRule,
				RuleID:   "rule-1",
			},
			responseStatus: http.StatusOK,
			responseBody: `{
				"id": "rule-2",
				"domainType": "rule",
				"extensions": {
					"ruleset": "host_label_rules",
					"folder": "/",
					"folder_index": 0,
					"properties": {"description": "Other", "disabled": false},
					"value_raw": "{}",
					"conditions": {}
				}
			}`,
			expectFolder: "/",
			expectIndex:  0,
		},
		{
			name:   "not found",
			ruleID: "missing",
			request: &RuleMoveRequest{
				Position: MovePositionBottomOfFolder,
				Folder:   "/",
			},
			responseStatus: http.StatusNotFound,
			responseBody:   `{"title": "Not Found", "status": 404, "detail": "Rule 'missing' not found"}`,
			expectError:    true,
			errorContains:  "not found",
		},
		{
			name:   "precondition failed",
			ruleID: "rule-1",
			request: &RuleMoveRequest{
				Position: MovePositionTopOfFolder,
				Folder:   "/",
			},
			etag:           `"stale"`,
			responseStatus: http.StatusPreconditionFailed,
			responseBody:   `{"title": "Precondition Failed", "status": 412, "detail": "ETag mismatch"}`,
			expectError:    true,
			errorContains:  "modified outside of Terraform",
		},
		{
			name:   "invalid position rejected by API",
			ruleID: "rule-1",
			request: &RuleMoveRequest{
				Position: "sideways",
			},
			responseStatus: http.StatusBadRequest,
			responseBody:   `{"title": "Invalid position", "status": 400, "detail": "Position 'sideways' is not a valid position."}`,
			expectError:    true,
			errorContains:  "not a valid position",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Errorf("Expected POST request, got %s", r.Method)
				}
				wantPath := "/check_mk/api/1.0/objects/rule/" + tt.ruleID + "/actions/move/invoke"
				if r.URL.Path != wantPath {
					t.Errorf("Expected path %s, got %s", wantPath, r.URL.Path)
				}

				// The move action requires If-Match (etag="input"); an empty
				// etag must fall back to "*"
				wantMatch := tt.etag
				if wantMatch == "" {
					wantMatch = "*"
				}
				if got := r.Header.Get("If-Match"); got != wantMatch {
					t.Errorf("Expected If-Match %q, got %q", wantMatch, got)
				}

				var req RuleMoveRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("Failed to decode request body: %v", err)
				}
				if req.Position != tt.request.Position {
					t.Errorf("Expected position %s, got %s", tt.request.Position, req.Position)
				}
				if req.Folder != tt.request.Folder {
					t.Errorf("Expected folder %q, got %q", tt.request.Folder, req.Folder)
				}
				if req.RuleID != tt.request.RuleID {
					t.Errorf("Expected rule_id %q, got %q", tt.request.RuleID, req.RuleID)
				}
				// folder must be omitted for before/after modes
				if tt.request.Folder == "" {
					var raw map[string]interface{}
					body, _ := json.Marshal(req)
					_ = json.Unmarshal(body, &raw)
					if _, ok := raw["folder"]; ok {
						t.Error("folder must be omitted from the move request body when empty")
					}
				}

				w.WriteHeader(tt.responseStatus)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := &Client{
				HTTPClient: server.Client(),
				BaseURL:    mustParseURL(server.URL),
			}

			rule, err := client.MoveRule(context.Background(), tt.ruleID, tt.request, tt.etag)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if rule == nil {
					t.Fatal("Expected rule but got nil")
				}
				if rule.ID != tt.ruleID {
					t.Errorf("Expected ID %q, got %q", tt.ruleID, rule.ID)
				}
				if rule.Extensions.Folder != tt.expectFolder {
					t.Errorf("Expected folder %q, got %q", tt.expectFolder, rule.Extensions.Folder)
				}
				if rule.Extensions.FolderIndex != tt.expectIndex {
					t.Errorf("Expected folder_index %d, got %d", tt.expectIndex, rule.Extensions.FolderIndex)
				}
			}
		})
	}
}
