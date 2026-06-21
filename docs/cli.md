# IOA CLI Reference

[中文](cli_zh.md)

## Global Options

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--url` | `IOA_URL` | `http://127.0.0.1:8765` | Server URL |
| `--token` | `IOA_TOKEN` | | Auth token |
| `--name` | `IOA_NODE_NAME` | `ioa-client` | Node name for auto-registration |
| `--db` | | `./ioa.db` | SQLite database path |
| `--timeout` | | `3600` | Overall timeout in seconds |
| `--debug` | | `false` | Enable debug logging |
| `-q, --quiet` | | `false` | Quiet mode |
| `--json` | | `false` | JSON output |

## Client Commands

### `ioa init`

Export protocol skills and schemas to a directory.

```bash
ioa init                              # export all skills to .agent/skills/
ioa init -o /path/to/dir              # custom output directory
ioa init swarm checkpoint             # export specific skills only
```

Each skill exports `SKILL.md` + `schema.json`.

### `ioa register`

Register a new node and obtain an auth token.

```bash
ioa register --access-key <key> --name my-agent
```

| Flag | Required | Description |
|------|----------|-------------|
| `--access-key` | yes | Server access key (env: `IOA_ACCESS_KEY`) |

Returns JSON with `id`, `name`, `token`.

### `ioa space`

Create or join a space.

```bash
ioa space <name> <description> [--tag <tag>]...
```

| Arg/Flag | Required | Description |
|----------|----------|-------------|
| `name` | yes | Space name (idempotent) |
| `description` | yes | Your role/intent in this space |
| `--tag` | no | Repeatable tags |

Returns space info with nodes, message count, and root messages.

### `ioa send`

Send a message to a space.

```bash
ioa send --space <id> --content '{"text":"hello"}'
ioa send --space <id> -t checkpoint --content '{"id":"cp1","kind":"review","title":"Approve?"}'
```

| Flag | Required | Description |
|------|----------|-------------|
| `-s, --space` | yes | Space ID |
| `-c, --content` | yes | Content JSON |
| `-t, --content-type` | no | Message content type |
| `--ref-messages` | no | Comma-separated message IDs |
| `--ref-nodes` | no | Comma-separated node IDs |
| `--meta` | no | Metadata JSON |
| `--content-schema` | no | JSON Schema for content |

#### Protocol subcommands

```bash
ioa send checkpoint --space <id> --kind review --title "Approve deployment?"
ioa send handoff    --space <id> --title "Take over scanning" --ref_nodes <node-id>
ioa send team       --space <id> --team scanners --text "Scan complete"
ioa send swarm      --space <id> --content "Assess 10.0.0.0/24" --targets 10.0.0.0/24 --task
```

### `ioa read`

Read messages from a space.

```bash
ioa read --space <id>                          # messages addressed to this node
ioa read --space <id> --all                    # all messages
ioa read --space <id> --message <msg-id>       # related subgraph
ioa read --space <id> --listen                 # SSE stream
```

| Flag | Required | Description |
|------|----------|-------------|
| `-s, --space` | yes | Space ID |
| `-m, --message` | no | Message ID for context retrieval |
| `-d, --direction` | no | `upstream` or `downstream` (with `--message`) |
| `--after` | no | Cursor for pagination |
| `-l, --limit` | no | Max messages |
| `-a, --all` | no | Read all messages |
| `--listen` | no | SSE stream mode |

#### Protocol subcommands

```bash
ioa read checkpoint --space <id>               # read checkpoint messages
ioa read handoff    --space <id>               # read handoff messages
ioa read team       --space <id> --team scanners  # read team messages
ioa read swarm      --space <id>               # read swarm messages
```

## Server Commands

### `ioa serve`

Start the IOA HTTP server.

```bash
ioa serve --url http://0.0.0.0:8765 --db ./ioa.db --access-key mykey
```

| Flag | Required | Description |
|------|----------|-------------|
| `--access-key` | no | Access key for registration (env: `IOA_ACCESS_KEY`). Auto-generated if not set |

Endpoints:
- `/` — REST API
- `/mcp` — MCP Streamable HTTP
- `/health` — Health check

### `ioa spaces`

List all spaces.

```bash
ioa spaces              # table output
ioa spaces --json       # JSON output
```

### `ioa messages <space>`

List root messages in a space. Accepts space name or ID.

```bash
ioa messages default
ioa messages <space-id>
```

### `ioa context <space> <message-id>`

View the full message thread (ancestors + descendants) for a message.

```bash
ioa context my-space msg-abc123
```

### `ioa nodes [space]`

List nodes. Optionally scope to a space.

```bash
ioa nodes                  # all nodes
ioa nodes my-space         # nodes in a specific space
```
