package mcp

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
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
	return newServer(c).Run(ctx, &mcpsdk.StdioTransport{})
}

func runHTTP(ctx context.Context, args []string) error {
	flags := &resolvedFlags{}
	fs := flag.NewFlagSet("http", flag.ExitOnError)
	registerConnectionFlags(fs, flags)
	addr := fs.String("addr", ":8080", "address to listen on")
	path := fs.String("path", "/mcp", "path to mount the MCP endpoint on")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	return serveHTTP(ctx, flags, *addr, *path)
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

type clientContextKey struct{}

// serveHTTP serves the MCP endpoint and the health probes on one listener.
//
// The Streamable HTTP handler sits behind a per-request auth gate. The gate
// stashes a request-scoped client in the context for the SDK's getServer
// callback, which can't return an error, so a missing token must be rejected
// with 401 before the handler runs.
//
// /livez and /readyz are unauthenticated, since the gate wraps only the MCP
// handler rather than the mux. A deployment that exposes this listener to the
// internet should route the MCP path alone and leave the probes to whatever
// reaches the process directly.
func serveHTTP(ctx context.Context, flags *resolvedFlags, addr, path string) error {
	// A tool call runs as long as the upstream Chief API takes, so a WriteTimeout
	// would sever it mid-response.
	srv := &http.Server{
		Addr:              addr,
		Handler:           newHTTPHandler(flags, path),
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
func newHTTPHandler(flags *resolvedFlags, path string) http.Handler {
	// Stateless mode runs getServer on every request, so each request's own
	// X-API-Key reaches the tools; stateful mode would pin the initialize-time
	// client for the session's lifetime. Every tool here is plain
	// request/response, so losing SSE costs nothing.
	streamable := mcpsdk.NewStreamableHTTPHandler(func(r *http.Request) *mcpsdk.Server {
		c, _ := r.Context().Value(clientContextKey{}).(*chief.Client)
		return newServer(c)
	}, &mcpsdk.StreamableHTTPOptions{Stateless: true})

	mux := http.NewServeMux()
	mux.Handle(path, authMiddleware(flags, streamable))
	mux.HandleFunc("GET /livez", healthOK)
	mux.HandleFunc("GET /readyz", healthOK)
	return mux
}

func healthOK(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("ok"))
}

// authMiddleware rejects tokenless requests and passes a request-scoped client
// to the MCP handler via context.
func authMiddleware(flags *resolvedFlags, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := requestAPIKey(r)
		if apiKey == "" {
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

		ctx := context.WithValue(r.Context(), clientContextKey{}, c)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
