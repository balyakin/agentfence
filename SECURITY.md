# Security

AgentFence v1 protects against accidental exposure of local secrets, source Git history, `$HOME`, and unreviewed patch application when running local AI coding agents.

Hard-mode on Linux requires `bubblewrap`. Soft mode only scrubs environment and uses a shadow workspace; it is not a sandbox.

AgentFence does not protect against malicious code that a user reviews and applies, kernel or bubblewrap exploits, or deliberate user configuration that mounts secrets or allows network access.

Report security issues privately through the project maintainers. Do not include raw secrets in reports.
