# IOA Protocol Design

[中文](design_zh.md)

## Design Philosophy

### Semantic-First

The core idea: **state is conveyed through meaning, not through pre-defined structures.**

Traditional protocols pre-define a set of states (pending/running/complete) and force participants to pick one. This requires the protocol designer to anticipate every possible state at design time — impossible in practice. Each unanticipated state becomes a "patch" to the protocol.

IOA's approach: participants describe state, context, and intent in `content` using natural language or free-form structured data. Receivers (AI or human) extract information by understanding semantics. The protocol defines no business semantics — only the mechanism to transmit them.

This isn't laziness about schema — it's the correct use of AI's capability. When the receiver has semantic understanding, pre-defined structure is redundant constraint. When it doesn't, the optional `content_schema` provides structure at the application layer.

| Traditional Protocol | IOA |
|---------------------|-----|
| Pre-defined task states (5/10/20 states) | State is natural language in `content` — unlimited |
| Pre-defined fields (priority, deadline, assignee...) | Participants freely define fields in `content` |
| Fixed message types (request/response/notification) | Message type emerges from `content` and graph structure |
| Metadata carries business semantics | `content` carries all semantics; system fields are strictly non-semantic |

### Participant Equality

The protocol doesn't distinguish Agent from Human. Both are **Node** — equal participants.

Traditional systems cast Humans as "approvers" and Agents as "executors", hardcoding role asymmetry into the protocol layer. IOA's position: **the protocol only cares about address and message format, not whether the participant is carbon or silicon.**

A Human can initiate tasks like an Agent. An Agent can approve work like a Human. Capability differences (response speed, concurrency, expertise) are declared in metadata, not enforced by protocol.

### Minimalism

The entire protocol:

| Layer | Concepts | Count |
|-------|----------|-------|
| L0 (Infrastructure) | Space | 1 |
| L1 (Protocol) | Node, Message, Ref | 3 |
| L2 (Application) | Checkpoint, Team, Handoff, Swarm... | Emergent from L0+L1 |

Tools exposed to participants:

| Tool | Description |
|------|-------------|
| `ioa_space` | Declare entry into a collaboration domain |
| `ioa_send` | Write a message |
| `ioa_read` | Read messages |

Participants need to understand 3 core concepts and 3 tools. All complex interaction patterns (approval flows, parallel dispatch, DAG convergence, task handoff) compose from primitives — no additional protocol-level concepts needed.

### Mechanism vs. Policy

The protocol defines **mechanism**, not **policy**.

- **Mechanism**: Messages reference other Messages and Nodes through Refs, forming graph structure; Spaces provide isolation boundaries
- **Policy**: Whether to accept, how to handle timeout, how to compensate failure — decided by the application layer

### AI-First

Optimized for AI agent tool-use:

- **Small, complete tool closure**: `ioa_space`, `ioa_send`, `ioa_read` are sufficient for full participation
- **No implicit session state**: Message context lives in the Space's Message Graph, not in connection state
- **Composition over memory**: Complex patterns emerge from composing simple operations; AI doesn't need to remember special APIs

## Architecture

```
L0 (Infrastructure)  Space                               — isolation boundary, server-managed
L1 (Protocol)        Node    Message    Ref               — participants, communication, association
L2 (Application)     Checkpoint  Team  Handoff  Swarm ... — emergent patterns from L0+L1
```

- **L0** is infrastructure — isolation, authentication, Message Graph container. Managed by the server, transparent to participants.
- **L1** is the protocol core — participants (Node), communication units (Message), and their references (Ref). Participants interact directly with L1.
- **L2** is collaboration patterns — emerging from L0+L1 composition, not part of the protocol itself.

## Core Concepts

### Space (L0) — Isolation Boundary

Space is the Message Graph's container and isolation boundary.

```
Space { id: string, name: string }
```

**Why L0**: Space is a prerequisite for all interaction — without it, Messages have nowhere to be stored, Refs have nothing to point to. It's infrastructure, not something participants interact with directly.

**Key properties**:

- **Idempotent by name**: same name always returns the same Space
- **Isolation is unbreakable**: `refs.messages` cannot cross Space boundaries. Each Space's Message Graph is self-contained
- **Declarative join**: `ioa_space(name, description)` joins and declares intent; produces no Message, doesn't alter the Message Graph
- **No Space-to-Space relations**: the protocol doesn't define parent/child, bridge, or nesting between Spaces. Cross-Space communication happens via Nodes sending messages in different Spaces

**Semantic isolation**: when a Team discusses internally in Space B, their discussion doesn't pollute Space A's message graph. Space A only sees the final conclusion when a Node explicitly sends it there.

### Node (L1) — Participant

```
Node { id: string, name: string, meta: object }
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Globally unique, server-generated |
| `name` | string | yes | Human-readable name |
| `meta` | object | no | Arbitrary metadata, default `{}` |

- No distinction between Agent and Human — `meta.kind` is metadata, doesn't change protocol behavior
- Can exist in multiple Spaces simultaneously
- `description` is per-Space (declared via `ioa_space`), not a global Node property

### Message (L1) — Communication Unit

```
Message { id, sender, created_at, content, refs }
```

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Globally unique, server-generated |
| `sender` | string | Sender Node ID |
| `created_at` | string | Server write time, RFC3339 UTC |
| `content` | object | Arbitrary structured payload, **cannot be null** |
| `refs` | Ref | References |

**5 public fields only**. Implementations may maintain internal fields (`space_id`, append position) but must not expose them.

**Immutable**: once written, cannot be modified or deleted. Space is an append-only log.

### Ref (L1) — Connection Mechanism

```
Ref { messages: string[], nodes: string[] }
```

Two forms, one mechanism — pointer arrays on Messages:

| Form | Points to | Semantics |
|------|-----------|-----------|
| `refs.messages` | Message IDs (same Space) | Causal chain, builds graph structure |
| `refs.nodes` | Node IDs | Recipient routing |

Both are arrays: supporting DAG merges (multiple parents) and multiple recipients. Empty `[]` means no reference.

**Why only two forms**: Ref → Space is unnecessary. Space reachability is conveyed through `content` (e.g. including a Space ID in the message body). Adding Ref → Space would force the protocol to define "what does referencing a Space mean" — violating mechanism/policy separation.

### Message Graph — Emergent Structure

Messages form a directed graph through `refs.messages`. The protocol doesn't prescribe the shape — structure emerges from usage:

```
Root              Thread            Tree               DAG

  [M1]              [M1]             [M1]            [M1]  [M2]
                      ↑              ↗    ↖             ↖  ↗
                     [M2]         [M2]      [M3]         [M3]
                      ↑
                     [M3]
```

| Pattern | Formation | Use case |
|---------|-----------|----------|
| Root | `refs.messages = []` and `refs.nodes = []` | Public entry point |
| Thread | Each message refs exactly one parent | Linear conversation |
| Tree | Multiple messages ref the same parent | Task decomposition |
| DAG | One message refs multiple parents | Result aggregation |

**Context** = traversing all ancestors along `refs.messages` from any Message. New Nodes replay Message history to get full context — "state" at any moment is a projection of the Message Graph.

## Operations

### `ioa_space` — Create or Join

| Param | Type | Required |
|-------|------|----------|
| `name` | string | yes |
| `description` | string | yes |

1. Auto-register caller Node if not yet registered
2. Create or get Space by name (idempotent)
3. Add caller to Space, record `description` (can be updated)
4. Return Space state + root messages

### `ioa_send` — Send Message

| Param | Type | Required |
|-------|------|----------|
| `space_id` | string | yes |
| `content` | object | yes |
| `refs` | Ref | no |
| `content_schema` | object | no |

1. Auto-ensure Node is registered
2. `content` cannot be null
3. If `content_schema` exists: validate it's a legal JSON Schema; store as declarative metadata on the Message (does **not** validate any Message's content)
4. `refs` defaults to `{messages:[], nodes:[]}`
5. Validate refs (messages must exist in same Space; nodes must be registered)
6. Generate Message ID, append to Space
7. Return complete Message

### `ioa_read` — Read Messages

| Param | Type | Required |
|-------|------|----------|
| `space_id` | string | yes |
| `message_id` | string | no |
| `direction` | string | no |
| `after` | string | no |
| `limit` | int | no |
| `all` | bool | no |
| `listen` | bool | no |

**Read modes** (by priority):

| Condition | Returns |
|-----------|---------|
| `message_id` set | Related subgraph (ancestors + descendants) |
| `all = true` | All messages in Space |
| Caller identity known | Messages where `refs.nodes` includes caller |
| Default | Root messages |

**Direction** (with `message_id`): `upstream` (ancestors only), `downstream` (descendants only), omit for both.

**SSE streaming**: `listen = true` opens a long-lived connection pushing new messages.

**Pagination**: `after` (cursor) + `limit` (max count).

## Errors

| Category | Trigger |
|----------|---------|
| **not_found** | Node / Space / Message doesn't exist |
| **invalid_input** | Validation failure: empty name, null content, invalid refs, non-positive limit, invalid JSON Schema |
| **internal** | Implementation error |

## Comparison with Existing Protocols

| Dimension | MCP | A2A | IOA |
|-----------|-----|-----|-----|
| Core relation | Model ↔ Tool | Agent ↔ Agent | Node ↔ Space ↔ Node |
| Participants | Asymmetric (Host/Client/Server) | Asymmetric (Client/Remote) | **Fully equal** |
| Human participation | Out of scope | Out of scope | **First-class** |
| Multi-party | No | Bilateral | **Any number** |
| Graph structure | None | None | **Thread / Tree / DAG** |
| State model | Context window | Task object | **Append-only Message Graph** |
| Concept count | ~10 | ~8 | **4** |

## Theoretical Foundations

| Theory | Core Insight | IOA Correspondence |
|--------|-------------|-------------------|
| **π-calculus** (Milner 1992) | Channel names can be sent on channels | Space ID can be sent in Message `content` |
| **Actor Model** (Hewitt 1973) | Everything is an Actor; Create/Send/Become | Everything is a Node; ioa_space/ioa_send/ioa_read |
| **Event Sourcing** | Event sequence is the single source of truth | Message Graph is the single source of truth |
| **Lamport Causality** (1978) | Only causal relationships are reliable ordering | `refs.messages` establishes causal chains |
| **Graph Theory** | DAG topological sort ensures acyclic traversal | Context traversal = DAG ancestor topo-sort |
