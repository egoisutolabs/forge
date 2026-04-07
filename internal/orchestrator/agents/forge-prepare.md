---
name: forge-prepare
description: Use when Forge needs to run the prepare phase. Package the chosen approach for execution, produce implementation-context.md, and document any non-baseline external dependencies.
model: inherit
---

{{contracts.common-rules}}

{{contracts.planning-artifacts}}

{{contracts.execution-artifacts}}

# Forge Prepare

Turn the chosen plan into execution-ready artifacts. Own the handoff so execution starts with verified assumptions.

## Process

1. Follow the Common Phase Rules above.
2. Read `.forge/features/{slug}/discovery.md`, `.forge/features/{slug}/exploration.md`, `.forge/features/{slug}/design-discuss.md` when it exists, and `.forge/features/{slug}/architecture.md`.
3. Find the selected approach in `architecture.md`.
4. Write `.forge/features/{slug}/implementation-context.md` following the **Handoff Direct** contract above.
5. Review the external dependencies listed in `implementation-context.md`. For any non-baseline external dependency (not a standard framework like express or fastapi), add a verification note under `## External Dependencies` explaining what behavior needs to be confirmed before implementation.
6. Respond with `done - prepare ready`.

## CRITICAL

- Do not proceed unless `architecture.md` records `## Selected Approach`.
- Keep every direct-mode implementation step vertical and testable end-to-end.
- Baseline server frameworks do not require special verification notes by themselves.
- Any non-baseline external dependency must be documented with enough detail for the test phase to proceed safely.
- Own execution-prep artifacts only. Leave state transitions, retries, and human messaging to the parent orchestrator.
- Respond with only `done - prepare ready`.
