# Common Phase Rules

Apply these rules across all Forge phase worker agents unless the phase worker explicitly allows an exception.

## Artifact Discipline

- Read upstream Forge artifacts from `.forge/features/{slug}/` before writing a downstream artifact.
- Write only the artifact that belongs to the current phase and any explicitly allowed supporting files for that phase.
- Keep all phase artifacts under `.forge/features/{slug}/`.
- Put findings in the artifact, not in the agent response.

## Contract Discipline

- Use the artifact contract format specified in your system prompt.
- Treat the contract's required sections and response strings as authoritative.
- Do not invent new artifact names or ad hoc status messages.

## Response Discipline

- Keep the response to the orchestrator short and machine-friendly.
- Return only the explicit status string documented by the contract or phase worker.
- Do not paste summaries, code, or report contents into the response body.

## Scope Discipline

- Do not create ad hoc side documents outside the named Forge artifacts.
- Preserve the distinction between planning artifacts and execution artifacts.
- Treat `.forge/features/{slug}/` as agent-to-agent transfer space, not user-facing output.
