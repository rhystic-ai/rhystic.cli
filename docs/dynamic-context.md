# Dynamic Context Passing

## Problem

Pipelines use different agent roles (researcher, developer, reviewer, etc.) but there's no way to inject runtime context into them. Each node gets a static prompt from the DOT file. Real workflows need external input: a topic to research, a ticket to implement, constraints to follow.

## Idea

A `--context` / `-c` flag that feeds dynamic data into the pipeline. Context flows through nodes and gets prepended to each node's prompt, scoped by what that role needs.

```bash
attractor run pipeline.dot -c "topic=Best mono-black Modern MTG deck" -c @constraints.md
attractor agent -r researcher -c "topic=Mono-black Modern" "Research this topic"
```

## How It Works

1. CLI collects `-c` values (repeatable). `@path` reads a file, plain strings are literal.
2. Context gets stored in the pipeline's `pcontext.Context` under a `user.*` namespace.
3. Node prompts can reference context via `$context.topic` or `$context` (dump all).
4. Each role's system prompt template can also access context via `{{.Context}}`.

## Context Flow in Pipelines

```
User input (-c flags)
    |
    v
pcontext.Context["user.topic"] = "Mono-black Modern"
pcontext.Context["user.constraints"] = <file contents>
    |
    v
Node prompt expansion: "$context.topic" -> "Mono-black Modern"
    |
    v
Role system prompt: "{{.Context}}" -> rendered context block
```

Nodes also write back to context (they already do via `WithContextUpdate`), so downstream nodes see accumulated results plus original user context.

## What Changes

- `options` struct: add `context []string`
- `parseArgs`: collect `-c` / `--context` (repeatable)
- `runAgent`: resolve `@file` refs, build context preamble, prepend to prompt
- `runPipeline`: same resolution, inject into `pcontext.Context` under `user.*` keys
- `CodergenHandler`: expand `$context.*` vars in node prompts (same pattern as `$goal`)
- Role templates: add `{{.Context}}` to `TemplateData`

## Not in Scope (Yet)

- Per-node context filtering (all nodes see all context for now)
- Structured context schemas or validation
- Context from external sources (URLs, APIs, DB queries)
