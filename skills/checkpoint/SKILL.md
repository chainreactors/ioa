---
name: checkpoint
description: Human-in-the-loop review protocol. Use when execution reaches an approval boundary that requires human judgment before proceeding.
---

# Checkpoint

Pause execution and submit an artifact for human review. Execution resumes only after the reviewer responds.

## Message format

Send with `content_type: "checkpoint"` on the message envelope.

Content body:

```json
{
  "id": "<unique-id>",
  "title": "Short description",
  "content": "Markdown body with full context for the reviewer",
  "options": ["Approve", "Reject"]
}
```

`id` is required and must be unique. Optional fields: `kind`, `target`, `status`.

Response — the reviewer sends this (with `refs.messages` pointing to your checkpoint):

```json
{
  "option": "Approve",
  "feedback": "Free-form comment from reviewer"
}
```

The response has **no `content_type`**. It is identified by the `refs.messages` relationship, not by content type.

## Mechanism

- `"Reject"` triggers mechanism-level rollback. All other options are semantic guidance.
- Submission blocks your session until feedback arrives.

## When to use

- Destructive or irreversible action ahead
- Plan needs approval before execution
- Human preference matters between alternatives

Do NOT checkpoint trivial decisions.

## After receiving feedback

Read both the option and the feedback text. The option is shorthand; the text carries nuance.

- Approve → proceed
- Reject → stop, roll back
- Custom option → interpret and adjust
