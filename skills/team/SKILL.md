---
name: team
description: Named-group communication within a workspace. Use when agents need to broadcast to a specific group or discover peers.
---

# Team

Broadcast messages within named groups. Team is a filter label on the shared workspace space — NOT a separate space.

## Message format

Send with `content_type: "team"` on the message envelope.

Content body:

```json
{
  "team": "scanners",
  "text": "Scan complete. Found 3 vulnerabilities."
}
```

Both `team` and `text` are required.

## Tools

```
team_create(name, description)     — create or join (idempotent)
team_send(team, message)           — broadcast to group
team_read(team, limit=20)          — read recent messages
team_members(team)                 — list members
team_discover(role, service)       — find agents by capability
```

## When to use

- Broadcasting status or findings to a group
- Discovering available peers

Use **handoff** for 1:1 delegation. Use **swarm** for emergent coordination without pre-defined groups.
