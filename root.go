package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/Storytell-ai/chief-go/chief"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	return newServer(c).Run(ctx, &mcp.StdioTransport{})
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

// serveHTTP mounts the Streamable HTTP handler behind a per-request auth gate.
// The gate stashes a request-scoped client in the context for the SDK's
// getServer callback, which can't return an error, so a missing token must be
// rejected with 401 before the handler runs.
func serveHTTP(ctx context.Context, flags *resolvedFlags, addr, path string) error {
	streamable := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		c, _ := r.Context().Value(clientContextKey{}).(*chief.Client)
		return newServer(c)
	}, nil)

	mux := http.NewServeMux()
	mux.Handle(path, authMiddleware(flags, streamable))

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	fmt.Fprintf(os.Stderr, "chief-mcp listening on %s%s\n", addr, path)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
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
