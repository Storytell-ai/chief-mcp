package mcp

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestGetSessionTranscriptPagesThroughTheAPI(t *testing.T) {
	var mu sync.Mutex
	var gotPath, gotAfter, gotLimit, gotBefore string

	chiefAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.EscapedPath()
		gotAfter = r.URL.Query().Get("after_id")
		gotLimit = r.URL.Query().Get("limit")
		gotBefore = r.URL.Query().Get("before_id")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[`+
			`{"index":11,"speaker":0,"speaker_label":"You","source":"mic","text":"first","start":60,"end":62,"at":"1:00"},`+
			`{"index":13,"speaker":1,"speaker_label":"Speaker 1","source":"system","text":"second","start":63,"end":65,"at":"1:03"}`+
			`],"first_id":"11","last_id":"13","has_more":true}`)
	}))
	defer chiefAPI.Close()

	mcpServer := httptest.NewServer(newHTTPHandler(&resolvedFlags{baseURL: chiefAPI.URL}, "/mcp"))
	defer mcpServer.Close()
	mcpURL := mcpServer.URL + "/mcp"

	const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"chief-mcp-test","version":"0"}}}`
	initResp := postRPC(t, mcpURL, "key-a", "", initBody)
	sessionID := initResp.Header.Get("Mcp-Session-Id")
	drain(initResp)

	callBody := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"` + toolGetSessionTranscript +
		`","arguments":{"session_id":"sess/1","after_id":"10","limit":2}}}`
	callResp := postRPC(t, mcpURL, "key-b", sessionID, callBody)
	if callResp.StatusCode != http.StatusOK {
		t.Fatalf("tools/call: got status %d", callResp.StatusCode)
	}
	raw := rpcPayload(t, callResp)

	mu.Lock()
	defer mu.Unlock()
	if gotPath != "/v1/sessions/sess%2F1/transcript" {
		t.Fatalf("path = %q, want the session ID escaped into /v1/sessions/sess%%2F1/transcript", gotPath)
	}
	if gotAfter != "10" || gotLimit != "2" || gotBefore != "" {
		t.Fatalf("query after_id=%q limit=%q before_id=%q, want after_id=10 limit=2 and no before_id", gotAfter, gotLimit, gotBefore)
	}

	var rpc struct {
		Result struct {
			IsError           bool `json:"isError"`
			StructuredContent struct {
				Data []struct {
					Index int    `json:"index"`
					Text  string `json:"text"`
					At    string `json:"at"`
				} `json:"data"`
				LastID  string `json:"last_id"`
				HasMore bool   `json:"has_more"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &rpc); err != nil {
		t.Fatalf("decode tools/call response %s: %v", raw, err)
	}
	if rpc.Result.IsError {
		t.Fatalf("tools/call returned an error result: %s", raw)
	}
	page := rpc.Result.StructuredContent
	if len(page.Data) != 2 || page.Data[1].Index != 13 || page.Data[1].Text != "second" || page.Data[1].At != "1:03" {
		t.Fatalf("structured turns = %+v, want the two turns from the API", page.Data)
	}
	if page.LastID != "13" || !page.HasMore {
		t.Fatalf("last_id = %q, has_more = %t; want 13, true", page.LastID, page.HasMore)
	}
}

// rpcPayload returns the JSON-RPC message from a tools/call response, which the
// streamable handler sends either as plain JSON or as a single SSE event.
func rpcPayload(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer drain(resp)
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read response: %v", err)
		}
		return body
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			return []byte(data)
		}
	}
	t.Fatalf("no data event in SSE response: %v", scanner.Err())
	return nil
}
