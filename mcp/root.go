package mcp

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Storytell-ai/chief-go/chief"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// resolvedFlags holds the connection flags; empty fields default to CHIEF_* env
// vars inside chief.New.
type resolvedFlags struct {
	apiKey   string
	project  string
	baseURL  string
	insecure bool
	debug    bool
}

// Execute runs the subcommand args[0] names (stdio, http, version or help) and
// returns once it ends; the transports serve until ctx is cancelled.
func Execute(ctx context.Context, args []string) error {
	if len(args) < 1 {
		printUsage()
		return errors.New("no command provided")
	}

	cmd := args[0]
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		printUsage()
		return nil
	}

	switch cmd {
	case "stdio":
		return runStdio(ctx, args[1:])
	case "http":
		return runHTTP(ctx, args[1:])
	case "version", "-v", "--version":
		fmt.Println(buildVersion())
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func registerConnectionFlags(fs *flag.FlagSet, flags *resolvedFlags) {
	fs.StringVar(&flags.apiKey, "api-key", "", "Chief API key (env CHIEF_API_KEY)")
	fs.StringVar(&flags.project, "project", "", "project ID (env CHIEF_PROJECT_ID)")
	fs.StringVar(&flags.baseURL, "base-url", "", "API base URL (env CHIEF_BASE_URL; default https://api.storytell.ai)")
	fs.BoolVar(&flags.insecure, "insecure", false, "skip TLS certificate verification (local dev only)")
	fs.BoolVar(&flags.debug, "debug", false, "dump HTTP requests and responses")
}

func runStdio(ctx context.Context, args []string) error {
	flags := &resolvedFlags{}
	fs := flag.NewFlagSet("stdio", flag.ExitOnError)
	registerConnectionFlags(fs, flags)
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	c, err := newClient(flags)
	if err != nil {
		return err
	}
	return newStdioServer(c).Run(ctx, &mcpsdk.StdioTransport{})
}

// newStdioServer lists every tool: a stdio caller runs on its own personal
// access token, which carries write.
func newStdioServer(c *chief.Client) *mcpsdk.Server {
	s := newServer(c, toolWrite)
	registerUploadTool(s)
	return s.mcp
}

func runHTTP(ctx context.Context, args []string) error {
	flags := &resolvedFlags{}
	fs := flag.NewFlagSet("http", flag.ExitOnError)
	registerConnectionFlags(fs, flags)
	addr := fs.String("addr", ":8080", "address to listen on")
	path := fs.String("path", "/mcp", "path to mount the MCP endpoint on")
	authURL := fs.String("auth-url", "", "Chief auth service URL that a 401 sends clients to for sign-in (env CHIEF_AUTH_URL)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if *authURL == "" {
		*authURL = os.Getenv("CHIEF_AUTH_URL")
	}

	return serveHTTP(ctx, flags, *addr, *path, resourceMetadataURL(*authURL, *path))
}

// resourceMetadataURL is where the auth service serves this server's protected
// resource metadata: RFC 9728 puts the resource's path after the well-known
// suffix. Empty when no auth service is configured, as for a PAT-only server.
func resourceMetadataURL(authURL, path string) string {
	if authURL == "" {
		return ""
	}
	return strings.TrimRight(authURL, "/") + "/.well-known/oauth-protected-resource" + path
}

// newClient builds a Client from the resolved flags.
func newClient(flags *resolvedFlags) (*chief.Client, error) {
	return chief.New(
		chief.WithAPIKey(flags.apiKey),
		chief.WithProjectID(flags.project),
		chief.WithBaseURL(flags.baseURL),
		chief.WithInsecureSkipTLSVerify(flags.insecure),
		chief.WithDebug(flags.debug),
	)
}

type serverContextKey struct{}

// serveHTTP serves the MCP endpoint and the health probes on one listener.
//
// The Streamable HTTP handler sits behind a per-request auth gate. The gate
// stashes a request-scoped server in the context for the SDK's getServer
// callback, which can't return an error, so a bad token must be rejected
// before the handler runs.
//
// /livez and /readyz are unauthenticated, since the gate wraps only the MCP
// handler rather than the mux. A deployment that exposes this listener to the
// internet should route the MCP path alone and leave the probes to whatever
// reaches the process directly.
func serveHTTP(ctx context.Context, flags *resolvedFlags, addr, path, resourceMetadataURL string) error {
	// A tool call runs as long as the upstream Chief API takes, so a WriteTimeout
	// would sever it mid-response.
	srv := &http.Server{
		Addr:              addr,
		Handler:           newHTTPHandler(flags, path, resourceMetadataURL),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "chief-mcp listening on %s\n", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errc:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	return serveErr
}

// newHTTPHandler mounts the auth-gated MCP endpoint and the health probes on
// one mux.
func newHTTPHandler(flags *resolvedFlags, path, resourceMetadataURL string) http.Handler {
	// Stateless mode runs getServer on every request, so each request's own
	// X-API-Key reaches the tools; stateful mode would pin the initialize-time
	// client for the session's lifetime. Every tool here is plain
	// request/response, so losing SSE costs nothing.
	streamable := mcpsdk.NewStreamableHTTPHandler(func(r *http.Request) *mcpsdk.Server {
		s, _ := r.Context().Value(serverContextKey{}).(*mcpsdk.Server)
		return s
	}, &mcpsdk.StreamableHTTPOptions{Stateless: true})

	mux := http.NewServeMux()
	mux.Handle(path, authMiddleware(flags, resourceMetadataURL, streamable))
	mux.HandleFunc("GET /livez", healthOK)
	mux.HandleFunc("GET /readyz", healthOK)
	return mux
}

func healthOK(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("ok"))
}

// authMiddleware checks the request's credential against the Chief API and
// passes the MCP handler, via context, a server listing only the tools that
// credential may use.
//
// A 401 carries resource_metadata so an MCP client can find where to sign in.
// Only a missing or rejected credential is a 401: Claude answers one by asking
// the user to reconnect, so a Chief API outage is a 503 instead.
func authMiddleware(flags *resolvedFlags, resourceMetadataURL string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := requestAPIKey(r)
		if apiKey == "" {
			w.Header().Set("WWW-Authenticate", bearerChallenge(resourceMetadataURL, ""))
			http.Error(w, "missing API key: set X-API-Key or Authorization: Bearer <token>", http.StatusUnauthorized)
			return
		}

		project := r.Header.Get("X-Project-Id")
		if project == "" {
			project = flags.project
		}

		reqFlags := *flags
		reqFlags.apiKey = apiKey
		reqFlags.project = project
		c, err := newClient(&reqFlags)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		access, err := tokenAccess(r.Context(), c)
		if chief.IsUnauthorized(err) {
			w.Header().Set("WWW-Authenticate", bearerChallenge(resourceMetadataURL, "invalid_token"))
			http.Error(w, "invalid or expired API key", http.StatusUnauthorized)
			return
		}
		if err != nil {
			http.Error(w, "the Chief API is unavailable; retry shortly", http.StatusServiceUnavailable)
			return
		}

		ctx := context.WithValue(r.Context(), serverContextKey{}, newServer(c, access).mcp)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// tokenTimeout bounds only the gate's credential check, not the tool calls
// behind it, which run as long as the Chief API takes.
const tokenTimeout = 10 * time.Second

// tokenAccess asks the Chief API what the credential may do. A credential
// without the write scope, whatever else it holds, gets read tools only.
func tokenAccess(ctx context.Context, c *chief.Client) (toolAccess, error) {
	ctx, cancel := context.WithTimeout(ctx, tokenTimeout)
	defer cancel()
	info, err := c.Token(ctx)
	if err != nil {
		return 0, err
	}
	if slices.Contains(info.Scopes, chief.TokenScopeWrite) {
		return toolWrite, nil
	}
	return toolRead, nil
}

// bearerChallenge is the WWW-Authenticate value for a 401. errCode is an RFC
// 6750 error code, empty when the request sent no credential.
func bearerChallenge(resourceMetadataURL, errCode string) string {
	var params []string
	if errCode != "" {
		params = append(params, `error="`+errCode+`"`)
	}
	if resourceMetadataURL != "" {
		params = append(params, `resource_metadata="`+resourceMetadataURL+`"`)
	}
	if len(params) == 0 {
		return "Bearer"
	}
	return "Bearer " + strings.Join(params, ", ")
}

// requestAPIKey reads the token from X-API-Key, falling back to a Bearer token
// in the Authorization header. X-API-Key wins when both are set.
func requestAPIKey(r *http.Request) string {
	if key := r.Header.Get("X-API-Key"); key != "" {
		return key
	}
	if auth := r.Header.Get("Authorization"); auth != "" {
		if token, ok := strings.CutPrefix(auth, "Bearer "); ok {
			return strings.TrimSpace(token)
		}
	}
	return ""
}

func printUsage() {
	fmt.Println(`chief-mcp exposes the Chief public API to external agents as Model Context Protocol tools.

Usage:
  chief-mcp <command> [flags]

Commands:
  stdio   Serve the MCP tools over stdio for a local agent
  http    Serve the MCP tools over Streamable HTTP for remote agents
  version Print the version and exit`)
}
