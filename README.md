# Chief MCP Server

[![Release](https://img.shields.io/github/v/release/Storytell-ai/chief-mcp)](https://github.com/Storytell-ai/chief-mcp/releases)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)

An MCP server for [Chief](https://chief.bot/). Search your project's documents, and manage assets, labels, actions, live sessions, skills, and memories in your Chief project — directly from any MCP client like Claude Desktop, Cursor, or Claude Code.

It is a single static Go binary built on the [Chief Go SDK](https://github.com/Storytell-ai/chief-go), and talks only to the Chief public REST API.

Connect to the hosted server at `https://mcp.chief.bot/mcp`, or run the binary yourself over stdio or HTTP.

## Features

- **Chats** — Open a chat and send follow-up turns, optionally waiting for the response; list, get, rename, and delete chats; list, get, and delete individual messages; set a chat's visibility, manage a restricted chat's audience, and create, rotate, or revoke public share links.
- **Assets** — Upload local files as assets (stdio only), optionally waiting for ingest to finish; list, get, update, and delete them.
- **Search** — Ask a question about the project's documents and get back the matching passages, best first. Searches the whole project by default, or narrow it to given assets, labels, views, or concepts. Requires a paid plan.
- **Labels** — Create, list, get, update, and delete labels; attach and detach them on assets by name.
- **Actions** — Create scheduled or event-triggered actions with optional email outcomes; list, get, update, delete, enable, and disable them.
- **Live Sessions** — List, get (with the lifecycle state and the live summary's decisions and to-dos), update, and delete chat sessions; read the transcript page by page, from the start or the most recent turns.
- **Skills** — Create, list, get, update, and delete skills and personas; enable or disable them for the caller.
- **Memories** — Create, list, get, update, and delete project memories.
- **Projects** — List, create, and update projects; list members; create and revoke invitations. These tools need only the API key, not a project ID.

## Remote MCP Server

Chief hosts the server at `https://mcp.chief.bot/mcp`. Nothing to install, no local process to run — which suits web-based clients and anything that can reach a Streamable HTTP endpoint.

You sign in with your Chief account. When you connect, your client opens Chief's sign-in page and then a consent screen, where you pick one organization and choose whether the client can create and change data. A read-only connection doesn't list the write tools. Project-scoped tools take a `project_id` argument, and `list_projects` returns the projects in the connected organization. Manage and revoke connections in Chief under Settings › Connected apps. The full guide is at [chief.bot/mcp](https://chief.bot/mcp).

The hosted server doesn't offer `upload_file`: it reads a path on the machine running the server, so only the stdio transport registers it.

### Claude

Open Settings › Connectors › Add custom connector and enter `https://mcp.chief.bot/mcp`, then select Connect and sign in. Once Chief is listed in Claude's connector directory, you can add it from there instead.

### Claude Code

```bash
claude mcp add --transport http chief https://mcp.chief.bot/mcp
```

Then run `/mcp` in Claude Code, select `chief`, and authenticate.

### With a Personal Access Token

Scripts and clients that can't sign in through a browser send a Chief Personal Access Token with every request instead. Token requests have no default project, so project-scoped tools also need an `X-Project-Id` header. The Projects tools work with the token alone.

#### Claude Code

```bash
claude mcp add chief --transport http https://mcp.chief.bot/mcp \
  --header "X-API-Key: <api-key>" \
  --header "X-Project-Id: <project-id>"
```

#### Cursor

Open the command palette and choose "Cursor Settings" > "MCP" > "Add new global MCP server".

```json
{
  "mcpServers": {
    "chief": {
      "url": "https://mcp.chief.bot/mcp",
      "headers": {
        "X-API-Key": "<api-key>",
        "X-Project-Id": "<project-id>"
      }
    }
  }
}
```

Any client that speaks Streamable HTTP can use the same URL and headers. Clients that can only set an `Authorization` header may send `Authorization: Bearer <api-key>` instead; `X-API-Key` wins when both are present.

## Setup

Running the binary yourself takes a Chief Personal Access Token, sent as the API key. Most tools are project-scoped and also need a project ID. Credentials are passed as flags or read from the environment:

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

The server has two transports, selected by subcommand: **stdio** for a local agent, and **http** for remote agents. Run `chief-mcp` with no subcommand to print usage. `upload_file` is registered on stdio only, since it reads files from the machine running the server.

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

This is the transport the hosted server exposes; run it yourself to keep the server inside your own network. Each client authenticates per request by passing its API key in the `X-API-Key` header, and selects a project with the `X-Project-Id` header.

Start the server:

```bash
chief-mcp http --addr :8080 --path /mcp
```

The server listens on `http://localhost:8080` and exposes the MCP endpoint at `/mcp` using Streamable HTTP.

#### Claude Code

```bash
claude mcp add chief --transport http http://localhost:8080/mcp \
  --header "X-API-Key: <api-key>" \
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
        "X-API-Key": "<api-key>",
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
- `--auth-url`: the Chief auth service URL; a 401 points clients to its protected resource metadata so they can sign in (env `CHIEF_AUTH_URL`)

> [!NOTE]
> In HTTP mode the server authenticates each request from its headers, so `--api-key` applies to stdio only. `--project` sets the default project for requests that omit `X-Project-Id`; a project-scoped tool's `project_id` argument overrides both. A read-only token gets only the read tools.

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
   - Add headers: `X-API-Key: <api-key>` and `X-Project-Id: <project-id>`.
   - Click **Connect**, then use "List tools" to verify the server is working.

## License

Apache 2.0 — see [LICENSE](LICENSE).
