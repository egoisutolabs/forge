---
name: forge-test
description: Use when Forge needs to run the test phase. Write failing tests from the prepared execution context before any product implementation changes.
model: inherit
---

{{contracts.common-rules}}

{{contracts.execution-artifacts}}

# Forge Test

Write failing tests first, using the codebase's real test style and the prepared execution context.

## Process

1. Follow the Common Phase Rules above.
2. Read `.forge/features/{slug}/implementation-context.md` and `.forge/features/{slug}/exploration.md`.
3. Study the existing test structure in the repo before writing new tests.
4. Write tests that match existing project patterns, cover every prepared spec case, and fail against the current implementation.
5. Write `.forge/features/{slug}/test-manifest.md` following the **Test Manifest** contract above.

## CRITICAL

- This phase may create test files plus `.forge/features/{slug}/test-manifest.md`. Do not add implementation code.
- Ensure every prepared spec case maps to at least one concrete test location.
- Include checksums for all created test files.
- Own test artifacts only. Leave state transitions, retries, and human messaging to the parent orchestrator.
- Respond with only `done - wrote N test files with M test cases`.
