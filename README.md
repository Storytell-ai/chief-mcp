# Chief MCP Server

[![Release](https://img.shields.io/github/v/release/Storytell-ai/chief-mcp)](https://github.com/Storytell-ai/chief-mcp/releases)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)

An MCP server for [Chief](https://chief.bot/). Manage assets, labels, actions, live sessions, skills, and memories in your Chief project — directly from any MCP client like Claude Desktop, Cursor, or Claude Code.

It is a single static Go binary built on the [Chief Go SDK](https://github.com/Storytell-ai/chief-go), and talks only to the Chief public REST API.

## Features

- **Assets** — Upload local files as assets, optionally waiting for ingest to finish; list, get, update, and delete them.
- **Labels** — Create, list, get, update, and delete labels; attach and detach them on assets by name.
- **Actions** — Create scheduled or event-triggered actions with optional email outcomes; list, get, update, delete, enable, and disable them.
- **Live Sessions** — List, get (with the full transcript), update, and delete chat sessions.
- **Skills** — Create, list, get, update, and delete skills and personas; enable or disable them for the caller.
- **Memories** — Create, list, get, update, and delete project memories.

## Setup

A Chief Personal Access Token is required; it is sent as the API key. Most tools are project-scoped and also need a project ID. Credentials are passed as flags or read from the environment:

| Flag | Environment variable | Required |
|------|----------------------|----------|
| `--api-key` | `CHIEF_API_KEY` | yes |
| `--project` | `CHIEF_PROJECT_ID` | for project-scoped tools |
| `--base-url` | `CHIEF_BASE_URL` | no (defaults to `https://api.storytell.ai`) |

## Install

Download a prebuilt binary for your platform from the [releases page](https://github.com/Storytell-ai/chief-mcp/releases), or install with the Go toolchain:

```bash
go install github.com/Storytell-ai/chief-mcp@latest
```

Or build from source with [Task](https://taskfile.dev/):

```bash
git clone https://github.com/Storytell-ai/chief-mcp.git
cd chief-mcp
task install   # builds and installs chief-mcp into your GOBIN
```

## Usage

The server has two transports, selected by subcommand: **stdio** (default, for a local agent) and **http** (for remote agents).

### Quick Setup

The `chief` CLI writes ready-to-paste MCP configuration for your client, filling in the installed binary path and your credentials:

```bash
chief mcp config claude   # Claude Code / Claude Desktop
chief mcp config codex    # Codex
```

For Claude it also prints the one-liner equivalent:

```bash
claude mcp add chief -- chief-mcp stdio
```

To configure a client by hand instead, use the snippets below.

### Stdio Transport

#### Claude Code

```bash
claude mcp add chief \
  -e CHIEF_API_KEY=<api-key> \
  -e CHIEF_PROJECT_ID=<project-id> \
  -- chief-mcp stdio
```

#### Cursor

Open the command palette and choose "Cursor Settings" > "MCP" > "Add new global MCP server".

```json
{
  "mcpServers": {
    "chief": {
      "command": "chief-mcp",
      "args": ["stdio"],
      "env": {
        "CHIEF_API_KEY": "<api-key>",
        "CHIEF_PROJECT_ID": "<project-id>"
      }
    }
  }
}
```

#### Claude Desktop

Open Claude Desktop settings > "Developer" tab > "Edit Config".

```json
{
  "mcpServers": {
    "chief": {
      "command": "chief-mcp",
      "args": ["stdio"],
      "env": {
        "CHIEF_API_KEY": "<api-key>",
        "CHIEF_PROJECT_ID": "<project-id>"
      }
    }
  }
}
```

#### Codex

Add to `~/.codex/config.toml`:

```toml
[mcp_servers.chief]
command = "chief-mcp"
args = ["stdio"]
env = { CHIEF_API_KEY = "<api-key>", CHIEF_PROJECT_ID = "<project-id>" }
```

### HTTP Transport

Run the server over HTTP for remote or web-based integrations. Each client authenticates per request by passing its API key as a Bearer token in the `Authorization` header (or in `X-API-Key`), and selects a project with the `X-Project-Id` header.

Start the server:

```bash
chief-mcp http --addr :8080 --path /mcp
```

The server listens on `http://localhost:8080` and exposes the MCP endpoint at `/mcp` using Streamable HTTP.

#### Claude Code

```bash
claude mcp add chief --transport http http://localhost:8080/mcp \
  --header "Authorization: Bearer <api-key>" \
  --header "X-Project-Id: <project-id>"
```

#### Cursor

Open the command palette and choose "Cursor Settings" > "MCP" > "Add new global MCP server".

```json
{
  "mcpServers": {
    "chief": {
      "url": "http://localhost:8080/mcp",
      "headers": {
        "Authorization": "Bearer <api-key>",
        "X-Project-Id": "<project-id>"
      }
    }
  }
}
```

### Options

Both transports accept:

- `--api-key`: Chief API key (env `CHIEF_API_KEY`)
- `--project`: project ID (env `CHIEF_PROJECT_ID`)
- `--base-url`: API base URL (env `CHIEF_BASE_URL`; default `https://api.storytell.ai`)
- `--insecure`: skip TLS certificate verification (local dev only)
- `--debug`: dump HTTP requests and responses

The `http` transport adds:

- `--addr`: address to listen on (default `:8080`)
- `--path`: path to mount the MCP endpoint on (default `/mcp`)

> [!NOTE]
> In HTTP mode the server authenticates each request from its headers, so `--api-key` applies to stdio only. `--project` sets the default project for requests that omit `X-Project-Id`.

## Local Development

1. Clone this project and build:

```bash
git clone https://github.com/Storytell-ai/chief-mcp.git
cd chief-mcp
task build   # outputs bin/chief-mcp
```

This project uses [Task](https://taskfile.dev/) for common workflows:

```bash
task build            # compile the server to bin/chief-mcp
task run -- stdio     # run the server (args after -- are passed through)
task test             # run the test suite
task lint             # run golangci-lint
task fmt              # format the code
task release          # cut a release with goreleaser
```

Run `task` with no arguments to list every available task.

2. To use the local build, point an MCP client at the absolute path to `bin/chief-mcp`:

**Claude Code (stdio):**

```bash
claude mcp add chief \
  -e CHIEF_API_KEY=<api-key> \
  -e CHIEF_PROJECT_ID=<project-id> \
  -- /absolute/path/to/chief-mcp/bin/chief-mcp stdio
```

**Cursor / Claude Desktop (stdio):**

```json
{
  "mcpServers": {
    "chief-dev": {
      "command": "/absolute/path/to/chief-mcp/bin/chief-mcp",
      "args": ["stdio"],
      "env": {
        "CHIEF_API_KEY": "<api-key>",
        "CHIEF_PROJECT_ID": "<project-id>"
      }
    }
  }
}
```

MCP servers are long-lived stdio processes that don't hot-reload. After rebuilding, restart the MCP client session to pick up the new build.

### Testing with MCP Inspector

#### Using Stdio Transport

1. Set your credentials:

   ```bash
   export CHIEF_API_KEY=<api-key>
   export CHIEF_PROJECT_ID=<project-id>
   ```

2. Start the inspector against the built binary:

   ```bash
   npx @modelcontextprotocol/inspector bin/chief-mcp stdio
   ```

3. In the browser (Inspector UI), click **Connect**, then use "List tools" to verify the server is working.

#### Using HTTP Transport

1. Start the HTTP server in one terminal:

   ```bash
   bin/chief-mcp http --addr :8080
   ```

2. Start the inspector in another terminal:

   ```bash
   npx @modelcontextprotocol/inspector
   ```

3. In the browser (Inspector UI):

   - Choose **Streamable HTTP** (connect to URL).
   - **URL:** `http://localhost:8080/mcp`
   - Add headers: `Authorization: Bearer <api-key>` and `X-Project-Id: <project-id>`.
   - Click **Connect**, then use "List tools" to verify the server is working.

## License

Apache 2.0 — see [LICENSE](LICENSE).
