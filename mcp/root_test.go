package mcp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestPerRequestAPIKeyReachesChiefAPI pins the auth contract: the X-API-Key on
// each JSON-RPC request is the key used to call the Chief API, not whatever key
// opened the session.
func TestPerRequestAPIKeyReachesChiefAPI(t *testing.T) {
	var mu sync.Mutex
	var listKey string
	chiefAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sessions" {
			mu.Lock()
			listKey = r.Header.Get("X-API-Key")
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[],"has_more":false}`)
	}))
	defer chiefAPI.Close()

	flags := &resolvedFlags{baseURL: chiefAPI.URL}
	mcpServer := httptest.NewServer(newHTTPHandler(flags, "/mcp"))
	defer mcpServer.Close()
	mcpURL := mcpServer.URL + "/mcp"

	const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"chief-mcp-test","version":"0"}}}`
	initResp := postRPC(t, mcpURL, "key-a", "", initBody)
	if initResp.StatusCode != http.StatusOK {
		t.Fatalf("initialize: got status %d", initResp.StatusCode)
	}
	sessionID := initResp.Header.Get("Mcp-Session-Id")
	drain(initResp)

	const callBody = `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_sessions","arguments":{}}}`
	callResp := postRPC(t, mcpURL, "key-b", sessionID, callBody)
	if callResp.StatusCode != http.StatusOK {
		t.Fatalf("tools/call: got status %d", callResp.StatusCode)
	}
	drain(callResp)

	mu.Lock()
	got := listKey
	mu.Unlock()
	if got != "key-b" {
		t.Fatalf("Chief API saw X-API-Key %q on list_sessions; want the per-request key %q", got, "key-b")
	}
}

func postRPC(t *testing.T, url, apiKey, sessionID, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("X-API-Key", apiKey)
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	return resp
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}
