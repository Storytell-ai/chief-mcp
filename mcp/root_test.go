package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
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

	flags := &resolvedFlags{baseURL: chiefAPI.URL, project: "project_a"}
	mcpServer := httptest.NewServer(newHTTPHandler(flags, "/mcp", ""))
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

// TestUploadFileIsStdioOnly pins upload_file to stdio: it opens a path on the
// server's own filesystem, so the HTTP transport must never list it.
func TestUploadFileIsStdioOnly(t *testing.T) {
	if !hasTool(listStdioTools(t), toolUploadFile) {
		t.Errorf("stdio tools lack %s", toolUploadFile)
	}
	if hasTool(listHTTPTools(t, `["read","write"]`), toolUploadFile) {
		t.Errorf("HTTP tools include %s", toolUploadFile)
	}
}

// TestAuthGate pins how the HTTP gate answers each credential outcome. An MCP
// client follows resource_metadata to find where to sign in, and Claude
// answers any 401 by asking the user to reconnect, so a Chief API outage must
// surface as a 503.
func TestAuthGate(t *testing.T) {
	const metadataURL = "https://auth.example/.well-known/oauth-protected-resource/mcp"
	for _, tc := range []struct {
		name          string
		apiKey        string
		tokenStatus   int
		wantStatus    int
		wantChallenge string
	}{
		{
			name:          "no credential",
			apiKey:        "",
			tokenStatus:   http.StatusOK,
			wantStatus:    http.StatusUnauthorized,
			wantChallenge: `Bearer resource_metadata="` + metadataURL + `"`,
		},
		{
			name:          "rejected token",
			apiKey:        "oat_expired",
			tokenStatus:   http.StatusUnauthorized,
			wantStatus:    http.StatusUnauthorized,
			wantChallenge: `Bearer error="invalid_token", resource_metadata="` + metadataURL + `"`,
		},
		{
			name:          "API outage",
			apiKey:        "oat_valid",
			tokenStatus:   http.StatusInternalServerError,
			wantStatus:    http.StatusServiceUnavailable,
			wantChallenge: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chiefAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.tokenStatus)
				_, _ = io.WriteString(w, `{"user_id":"user_1","organization_id":null,"scopes":["read","write"]}`)
			}))
			defer chiefAPI.Close()
			mcpServer := httptest.NewServer(newHTTPHandler(&resolvedFlags{baseURL: chiefAPI.URL}, "/mcp", metadataURL))
			defer mcpServer.Close()

			const listBody = `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
			resp := postRPC(t, mcpServer.URL+"/mcp", tc.apiKey, "", listBody)
			drain(resp)

			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			if got := resp.Header.Get("WWW-Authenticate"); got != tc.wantChallenge {
				t.Errorf("WWW-Authenticate = %q, want %q", got, tc.wantChallenge)
			}
		})
	}
}

// TestResourceMetadataURL pins the address the 401 points to against the route
// the auth service serves the metadata on.
func TestResourceMetadataURL(t *testing.T) {
	for _, tc := range []struct {
		name    string
		authURL string
		want    string
	}{
		{name: "auth service URL", authURL: "https://auth.example", want: "https://auth.example/.well-known/oauth-protected-resource/mcp"},
		{name: "trailing slash", authURL: "https://auth.example/", want: "https://auth.example/.well-known/oauth-protected-resource/mcp"},
		{name: "no auth service", authURL: "", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resourceMetadataURL(tc.authURL, "/mcp"); got != tc.want {
				t.Errorf("resourceMetadataURL = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestToolsFollowTokenScopes pins the scope filter: a token without write is
// offered exactly the tools annotated read-only, and a token with write gets
// every HTTP tool.
func TestToolsFollowTokenScopes(t *testing.T) {
	all := listHTTPTools(t, `["read","write"]`)
	if len(all) != 59 {
		t.Errorf("read+write token lists %d tools, want 59", len(all))
	}
	var wantRead []string
	for _, tool := range all {
		if tool.Annotations.ReadOnlyHint {
			wantRead = append(wantRead, tool.Name)
		}
	}

	var gotRead []string
	for _, tool := range listHTTPTools(t, `["read"]`) {
		gotRead = append(gotRead, tool.Name)
	}
	slices.Sort(wantRead)
	slices.Sort(gotRead)
	if !slices.Equal(gotRead, wantRead) {
		t.Errorf("read-only token lists %v, want only the read tools %v", gotRead, wantRead)
	}
}

// TestProjectIDOverridesConnectionProject checks that a tool's project_id
// reaches the API as the project header for that call alone.
func TestProjectIDOverridesConnectionProject(t *testing.T) {
	var mu sync.Mutex
	var gotProjects []string
	chiefAPI := fakeChiefAPI(t, `["read"]`, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotProjects = append(gotProjects, r.Header.Get("X-Project-Id"))
		mu.Unlock()
		_, _ = io.WriteString(w, `{"data":[],"has_more":false}`)
	})
	mcpServer := httptest.NewServer(newHTTPHandler(&resolvedFlags{baseURL: chiefAPI.URL, project: "project_default"}, "/mcp", ""))
	defer mcpServer.Close()

	callTool(t, mcpServer.URL+"/mcp", toolListSessions, `{"project_id":"project_override"}`)
	callTool(t, mcpServer.URL+"/mcp", toolListSessions, `{}`)

	mu.Lock()
	defer mu.Unlock()
	if want := []string{"project_override", "project_default"}; !slices.Equal(gotProjects, want) {
		t.Fatalf("X-Project-Id per call = %v, want %v", gotProjects, want)
	}
}

// TestProjectScopedToolWithoutProject checks the error a model sees when
// neither the connection nor the call names a project: it must say how to get
// one.
func TestProjectScopedToolWithoutProject(t *testing.T) {
	t.Setenv("CHIEF_PROJECT_ID", "")
	var mu sync.Mutex
	called := false
	chiefAPI := fakeChiefAPI(t, `["read"]`, func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		called = true
		mu.Unlock()
		_, _ = io.WriteString(w, `{}`)
	})
	mcpServer := httptest.NewServer(newHTTPHandler(&resolvedFlags{baseURL: chiefAPI.URL}, "/mcp", ""))
	defer mcpServer.Close()

	raw := callTool(t, mcpServer.URL+"/mcp", toolListSessions, `{}`)
	var rpc struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &rpc); err != nil {
		t.Fatalf("decode tools/call response %s: %v", raw, err)
	}
	if !rpc.Result.IsError || len(rpc.Result.Content) == 0 {
		t.Fatalf("want an error result, got %s", raw)
	}
	if text := rpc.Result.Content[0].Text; !strings.Contains(text, "project_id") || !strings.Contains(text, toolListProjects) {
		t.Errorf("error %q should name project_id and %s", text, toolListProjects)
	}
	mu.Lock()
	defer mu.Unlock()
	if called {
		t.Error("the tool called a project-scoped route with no project")
	}
}

// fakeChiefAPI answers the gate's token check with scopes, a JSON array, and
// hands every other request to next.
func fakeChiefAPI(t *testing.T, scopes string, next http.HandlerFunc) *httptest.Server {
	t.Helper()
	chiefAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/token" {
			_, _ = io.WriteString(w, `{"user_id":"user_1","organization_id":null,"scopes":`+scopes+`}`)
			return
		}
		next(w, r)
	}))
	t.Cleanup(chiefAPI.Close)
	return chiefAPI
}

func callTool(t *testing.T, mcpURL, name, arguments string) []byte {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + arguments + `}}`
	resp := postRPC(t, mcpURL, "key-a", "", body)
	if resp.StatusCode != http.StatusOK {
		drain(resp)
		t.Fatalf("tools/call %s: got status %d", name, resp.StatusCode)
	}
	return rpcPayload(t, resp)
}

func hasTool(tools []*mcpsdk.Tool, name string) bool {
	return slices.ContainsFunc(tools, func(tool *mcpsdk.Tool) bool { return tool.Name == name })
}

func listStdioTools(t *testing.T) []*mcpsdk.Tool {
	t.Helper()
	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
	ss, err := newStdioServer(nil).Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer func() { _ = ss.Close() }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "chief-mcp-test", Version: "0"}, nil)
	cs, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer func() { _ = cs.Close() }()

	res, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	return res.Tools
}

// listHTTPTools lists the HTTP tools offered to a token holding scopes, a JSON
// array.
func listHTTPTools(t *testing.T, scopes string) []*mcpsdk.Tool {
	t.Helper()
	chiefAPI := fakeChiefAPI(t, scopes, http.NotFound)
	mcpServer := httptest.NewServer(newHTTPHandler(&resolvedFlags{baseURL: chiefAPI.URL}, "/mcp", ""))
	defer mcpServer.Close()

	const listBody = `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	resp := postRPC(t, mcpServer.URL+"/mcp", "key-a", "", listBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tools/list: got status %d", resp.StatusCode)
	}
	raw := rpcPayload(t, resp)

	var rpc struct {
		Result mcpsdk.ListToolsResult `json:"result"`
	}
	if err := json.Unmarshal(raw, &rpc); err != nil {
		t.Fatalf("decode tools/list response %s: %v", raw, err)
	}
	if len(rpc.Result.Tools) == 0 {
		t.Fatalf("tools/list returned no tools: %s", raw)
	}
	return rpc.Result.Tools
}

func postRPC(t *testing.T, url, apiKey, sessionID, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
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
