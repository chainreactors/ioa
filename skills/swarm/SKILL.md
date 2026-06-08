---
name: swarm
description: Autonomous multi-agent coordination protocol. Commander posts objectives; 3-5 nodes self-organize into tactical squads through natural language.
---

# Swarm

A commander posts an objective to a shared IOA space. Nodes in the space self-organize into a tactical squad — dividing work, sharing findings, and converging on results. No micromanagement.

## Roles

- **Commander** (aide): posts objectives, monitors, mediates human review. Does NOT assign specific work to specific nodes.
- **Node** (aiscan): receives objectives, self-organizes with peers, executes, reports.

## Message format

Send with `content_type: "swarm"` on the message envelope. The content body has **NO `type` field**.

```json
{"content": "Natural language message"}
```

Objective broadcast (commander → space):

```json
{"content": "Full vulnerability assessment of 10.0.0.0/24", "targets": ["10.0.0.0/24"], "task": true}
```

No `refs.nodes` — this is a broadcast to the entire space. All idle nodes pick it up.

| Field | Required | Description |
|-------|----------|-------------|
| `content` | **yes** | Natural language |
| `targets` | no | Operational targets for tools |
| `task` | no | `true` → all idle nodes start working |

---

## Squad formation (what happens when an objective arrives)

When a `task:true` broadcast arrives, every idle node in the space starts a task. The squad self-organizes in 3 steps:

### Step 1: Read + Introduce (one round)

Each node reads the space and sends one introduction:

```
"Node scanner-01. Skills: gogo, spray. Strongest at web surface analysis."
```

### Step 2: Claim (one round)

After reading peer introductions, each node claims a work scope based on its strengths:

```
"I'll take web surface (spray + neutron) for the full /24."
```

Rules:
- Pick scope matching your strongest skills
- If a peer already claimed your preferred scope, pick the next best
- Duplicate claims: earlier message wins, later agent adapts
- One message. Don't negotiate. Claim and start.

**Typical 3-5 node squad split:**

| Nodes | Split strategy |
|-------|---------------|
| 3 | recon / web scan / network services |
| 4 | recon / web scan / network services / credential testing |
| 5 | passive recon / active recon / web scan / network services / exploitation |

### Step 3: Execute + Share

```
claim → execute → share findings as you go → read space → claim next scope → …
```

Every phase boundary is a read-write cycle. Don't go silent.

---

## Commander role

The commander does NOT assign specific work to specific nodes. It:

1. **Posts objectives**: broadcast `task:true` to the space
2. **Monitors**: reads the space to track squad progress
3. **Mediates HITL**: when a node says "need human confirmation", creates a checkpoint and relays the decision
4. **Evaluates convergence**: all scopes reported + no blockers = done

### Posting an objective

```json
{"content": "Objective: penetration test of 10.0.0.0/24. Find all web vulnerabilities, weak credentials, and misconfigurations. Report findings with severity.", "targets": ["10.0.0.0/24"], "task": true}
```

No `refs.nodes`. The squad handles the rest.

### Multiple squads

For large targets, create multiple spaces — one per squad, one per objective:

```
aide.workspace.squad-web     → 3 nodes → web vulnerability assessment
aide.workspace.squad-network → 3 nodes → network service enumeration
aide.workspace.squad-recon   → 2 nodes → passive intelligence gathering
```

---

## Node role

### Receiving an objective

When a broadcast `task:true` arrives, you start a task. Read the space, introduce yourself, claim scope, execute.

### Sharing findings (share immediately, don't batch)

```
"Found SQL injection on 10.0.0.5:8080/api/search. Blind time-based, 5s delay. Severity: high."
```

### Requesting human input

Describe what you need in natural language. The commander handles the review process.

```
"Need human confirmation: found default admin credentials on 10.0.0.5. Logging in may trigger lockout. Waiting for approval."
```

### Peer coordination

- Read before each phase — a peer may have found something that changes your approach
- If a peer is blocked, help if you can
- If a peer goes silent past their ETA, announce you're taking over their scope

---

## Convergence

The squad is done when:
1. All claimed scopes have completion reports
2. No unclaimed targets
3. No unresolved blockers or pending human input
4. No new findings in the last round

Last active node writes a summary for the commander.

## Anti-patterns

- Commander micromanaging (assigning specific work to specific nodes)
- Over-negotiating (>2 messages before working)
- Silent work (peers can't coordinate with silence)
- Hoarding findings (share as you produce)
- Nodes trying to create checkpoints directly (describe the need, let commander handle it)
