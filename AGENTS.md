# Project conventions

Implement the hardware operations assistant in Go with CloudWeGo Eino. Read
`docs/design/hardware-operations-agent-technical-design.md` for interfaces and runtime rules,
and the active local ticket for the scope of each change.

Verify QA-01 through the public HTTP flow and the external model adapter. Use a real
temporary store; simulate only external dependencies. Label deterministic outputs
as REPLAY and report real model integration separately.

## Agent skills

### Issue tracker

Work is tracked in local Markdown, one ticket per file. See `docs/agents/issue-tracker.md`.

### Triage labels

Use the default role names and explicit implementation status. See `docs/agents/triage-labels.md`.

### Domain docs

Use a single root glossary and `docs/adr/` when they exist. See `docs/agents/domain.md`.
