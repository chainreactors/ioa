# IOA Extension Guide

[中文](extension_zh.md)

L2 collaboration patterns are **not protocol extensions** — they are conventions on how to combine `content` structure and `refs`. Adding a new pattern requires zero changes to the IOA server or protocol.

## How L2 Works

Every L2 pattern ultimately calls the same L1 operation:

```
ioa_send(space_id, content, refs)
```

- `content` — arbitrary dict, structure defined by L2
- `refs.messages` — for causal chains (Checkpoint uses it, Handoff doesn't)
- `refs.nodes` — for routing (Handoff uses it, Team doesn't)

Each L2 pattern is just a convention on `content` structure + `refs` combination.

## Built-in Patterns

```
                    refs.messages    refs.nodes    content_schema
Checkpoint              ✓               ✗              optional
Team                    ✗               ✗              ✗
Handoff                 ✗               ✓              ✓
Swarm                   ✓               ✓              ✓
```

| Pattern | Core Mechanism | Typical Flow |
|---------|---------------|--------------|
| Checkpoint | Message pair via `refs.messages` | submit → feedback |
| Team | Shared Space, broadcast | send → all members read |
| Handoff | `refs.nodes` routing | send → target picks up |
| Swarm | Graph structure + routing | broadcast → self-organize → report |

## Extension Mechanism: Skill + Subcommand

IOA provides two mechanisms for extending L2:

### 1. Skills (for AI agents)

Skills are `SKILL.md` + `schema.json` pairs that teach AI agents how to use a collaboration pattern. They live in `skills/<name>/`.

**SKILL.md** — natural language instructions:

```markdown
---
name: handoff
description: Fire-and-forget work delegation.
---

# Handoff

Delegate work to another agent. Send the context and move on.

## Message format
Send with `content_type: "handoff"` on the message envelope.
...
```

**schema.json** — content structure definition:

```json
{
  "type": "object",
  "properties": {
    "title": {"type": "string"},
    "message": {"type": "string"}
  },
  "required": ["title"]
}
```

Skills are embedded in the binary and exported via `ioa init`:

```bash
ioa init                    # exports all skills to .agent/skills/
ioa init handoff swarm      # exports specific skills
```

### 2. Subcommands (for CLI)

Protocol subcommands extend `ioa send` and `ioa read` with pattern-specific flags and logic. They are Go packages that register via `protocols.Register()`.

## Adding a New L2 Pattern

### Step 1: Define the protocol package

Create `protocols/<name>/<name>.go`:

```go
package vote

import (
    "context"
    "fmt"
    "github.com/chainreactors/ioa/protocols"
)

func init() {
    protocols.Register(&protocols.Protocol{
        Name:        "vote",
        Description: "Simple voting protocol",
        Send: &protocols.Handler{
            Description: "Cast a vote",
            Flags:       &SendFlags{},
            Execute:     execSend,
        },
        Read: &protocols.Handler{
            Description: "Read votes",
            Flags:       &ReadFlags{},
            Execute:     execRead,
        },
    })
}

type SendFlags struct {
    PollID string `long:"poll-id" json:"poll_id" description:"Poll identifier"`
    Choice string `long:"choice" json:"choice" description:"Your vote"`
}

type ReadFlags struct {
    PollID string `long:"poll-id" json:"poll_id" description:"Filter by poll"`
}

func execSend(ctx context.Context, env *protocols.Env, args interface{}) (string, error) {
    var flags SendFlags
    protocols.ParseArgs(args, &flags)
    if flags.PollID == "" || flags.Choice == "" {
        return "", fmt.Errorf("vote: --poll-id and --choice are required")
    }

    content := map[string]interface{}{
        "poll_id": flags.PollID,
        "choice":  flags.Choice,
    }

    msg, err := env.Client.Send(ctx, env.SpaceID, protocols.SendMessage{
        ContentType: "vote",
        Content:     content,
    })
    if err != nil {
        return "", err
    }
    data, _ := json.MarshalIndent(msg, "", "  ")
    return string(data), nil
}

func execRead(ctx context.Context, env *protocols.Env, args interface{}) (string, error) {
    var flags ReadFlags
    protocols.ParseArgs(args, &flags)

    messages, err := env.Client.Read(ctx, env.SpaceID, protocols.ReadOptions{All: true})
    if err != nil {
        return "", err
    }

    var votes []protocols.Message
    for _, m := range messages {
        if protocols.MessageContentType(m) == "vote" {
            if flags.PollID == "" || m.Content["poll_id"] == flags.PollID {
                votes = append(votes, m)
            }
        }
    }
    data, _ := json.MarshalIndent(votes, "", "  ")
    return string(data), nil
}
```

### Step 2: Register the import

Add a blank import in `cmd/ioa/main.go`:

```go
import (
    _ "github.com/chainreactors/ioa/protocols/vote"
)
```

### Step 3: Add a skill (optional)

Create `skills/vote/SKILL.md`:

```markdown
---
name: vote
description: Simple voting protocol for group decisions.
---

# Vote

Cast and tally votes within a space.

## Message format
Send with `content_type: "vote"`.
Content: `{"poll_id": "...", "choice": "..."}`
```

Create `skills/vote/schema.json`:

```json
{
  "type": "object",
  "properties": {
    "poll_id": {"type": "string", "description": "Poll identifier"},
    "choice": {"type": "string", "description": "Your vote"}
  },
  "required": ["poll_id", "choice"]
}
```

### Result

After these steps:

```bash
# CLI subcommands work automatically
ioa send vote --space <id> --poll-id p1 --choice approve
ioa read vote --space <id> --poll-id p1

# Skill export works
ioa init vote
```

No changes to the IOA server, protocol, or any existing code.

## Protocol Registration API

```go
type Protocol struct {
    Name        string
    Description string
    Send        *Handler
    Read        *Handler
}

type Handler struct {
    Description string
    Flags       interface{}    // struct with go-flags tags
    Execute     func(ctx context.Context, env *Env, args interface{}) (string, error)
}

type Env struct {
    Client   ClientAPI
    SpaceID  string
    NodeName string
}
```

- `Flags` — a struct with `long` tags for go-flags parsing. CLI users pass these as `--flag value`
- `Execute` — receives the parsed flags via `args`. Use `protocols.ParseArgs(args, &flags)` to deserialize
- `Env` — provides access to the IOA client, current space, and node name

## Embedding Skills

Skills are embedded into the binary via `skills/embed.go` using Go's `embed` package. To add a new skill, place `SKILL.md` and `schema.json` in `skills/<name>/` — they are automatically included.
