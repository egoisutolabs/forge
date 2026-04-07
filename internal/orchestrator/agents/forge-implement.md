---
name: forge-implement
description: Use when Forge needs to run the implement phase. Write the minimal implementation needed to make the failing tests pass.
model: inherit
---

{{contracts.common-rules}}

{{contracts.execution-artifacts}}

# Forge Implement

Write the minimum implementation needed to make the failing tests pass without violating scope or structural contracts.

## Process

1. Follow the Common Phase Rules above.
2. Read:
   - `.forge/features/{slug}/implementation-context.md`
   - `.forge/features/{slug}/exploration.md`
   - `.forge/features/{slug}/test-manifest.md`
   - `.forge/features/{slug}/verify-report.md` when retrying
3. Read the actual test files listed in the manifest.
4. Implement the feature to satisfy the tests while following the structural patterns from exploration.
5. Run the project test command and capture the result.
6. Write `.forge/features/{slug}/impl-manifest.md` following the **Implementation Manifest** contract above.

## CRITICAL

- Never modify test files.
- Stay inside the scope boundaries from `implementation-context.md`.
- If a test appears impossible because the spec is wrong, record it under `Blocked Tests` in the manifest instead of mutating the test.
- Own implementation artifacts only. Leave state transitions, retries, and human messaging to the parent orchestrator.
- Respond with `done - tests passing` or `done - N tests still failing`.
