package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Storytell-ai/chief-go/chief"
)

// TestSearchSummaryCountsPassagesFromTheAPI decodes a response shaped like the
// API's and checks the summary reports its passage count, which comes from
// total_chunks rather than the number of grouped results.
func TestSearchSummaryCountsPassagesFromTheAPI(t *testing.T) {
	const body = `{"query":"revenue","duration_ms":7,"total_chunks":3,"partial":false,"results":[` +
		`{"asset_id":"a1","title":"Q1","relevance_score":0.9,"chunks":[` +
		`{"position":0,"score":0.9,"content":"x"},{"position":1,"score":0.8,"content":"y"},{"position":2,"score":0.7,"content":"z"}]}]}`

	var resp chief.SearchResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode search response: %v", err)
	}

	const want = "found 3 passage(s) across 1 document(s) in 7ms"
	if got := searchSummary(&resp); got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}

// TestSearchScopeOnTheWire pins the flattening in scopeFromSearchRequest.
//
// The API reads an absent scope as "the whole project" and a present-but-empty
// scope as "no knowledge at all". A regression that always sends the object
// would turn every unscoped search into zero results, and the tool would still
// answer 200 — so nothing else would catch it.
func TestSearchScopeOnTheWire(t *testing.T) {
	for _, tc := range []struct {
		name      string
		arguments string
		wantScope bool
		wantLabel string
	}{
		{
			name:      "no lists sends no scope",
			arguments: `{"query":"revenue"}`,
			wantScope: false,
		},
		{
			name:      "a label list sends a scope",
			arguments: `{"query":"revenue","label_ids":["label_x"]}`,
			wantScope: true,
			wantLabel: "label_x",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var body []byte

			chiefAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/search" {
					read, _ := io.ReadAll(r.Body)
					mu.Lock()
					body = read
					mu.Unlock()
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"query":"revenue","duration_ms":1,"total_chunks":0,"partial":false,"results":[]}`)
			}))
			defer chiefAPI.Close()

			mcpServer := httptest.NewServer(newHTTPHandler(&resolvedFlags{baseURL: chiefAPI.URL, project: "project_a"}, "/mcp", ""))
			defer mcpServer.Close()
			mcpURL := mcpServer.URL + "/mcp"

			const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"chief-mcp-test","version":"0"}}}`
			initResp := postRPC(t, mcpURL, "key-a", "", initBody)
			sessionID := initResp.Header.Get("Mcp-Session-Id")
			drain(initResp)

			callBody := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"` +
				toolSearchKnowledgeBase + `","arguments":` + tc.arguments + `}}`
			callResp := postRPC(t, mcpURL, "key-b", sessionID, callBody)
			if callResp.StatusCode != http.StatusOK {
				t.Fatalf("tools/call: got status %d", callResp.StatusCode)
			}
			drain(callResp)

			mu.Lock()
			got := body
			mu.Unlock()
			if len(got) == 0 {
				t.Fatal("the tool did not call /v1/search")
			}

			var sent struct {
				Query string `json:"query"`
				Scope *struct {
					LabelIDs []string `json:"label_ids"`
				} `json:"scope"`
			}
			if err := json.Unmarshal(got, &sent); err != nil {
				t.Fatalf("decode request body %q: %v", got, err)
			}

			if sent.Query != "revenue" {
				t.Fatalf("query = %q, want %q", sent.Query, "revenue")
			}
			if tc.wantScope != (sent.Scope != nil) {
				t.Fatalf("scope present = %v, want %v; body was %s", sent.Scope != nil, tc.wantScope, got)
			}
			if tc.wantLabel != "" {
				if len(sent.Scope.LabelIDs) != 1 || sent.Scope.LabelIDs[0] != tc.wantLabel {
					t.Fatalf("label_ids = %v, want [%s]", sent.Scope.LabelIDs, tc.wantLabel)
				}
			}
		})
	}
}
