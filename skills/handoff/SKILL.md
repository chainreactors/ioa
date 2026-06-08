---
name: handoff
description: Fire-and-forget work delegation. Use when work exceeds the current task's scope and should be transferred to another agent without waiting.
---

# Handoff

Delegate work to another agent. Send the context and move on — no acknowledgment, no callback, no tracking.

## Message format

Send with `content_type: "handoff"` on the message envelope.

Content body:

```json
{
  "title": "Short label for the delegated work",
  "message": "Detailed context: what you found, where to look, what to try"
}
```

Routing is in `refs.nodes`, not in content. Set `refs.nodes` to the target agent's node ID.

## When to use

- Work exceeds current task's scope or expertise
- A finding needs different tools to act on
- Passing pipeline output to the next stage

Use **checkpoint** instead when you need to pause and wait for a response.

## Writing the message

Brief the receiver like a colleague who just walked in:

- What you found (evidence, not conclusions)
- Where to look (targets, endpoints, parameters)
- What you already tried
- What tools might help

## Key rule

After sending, keep working. Do not wait for acknowledgment — it will never come.
