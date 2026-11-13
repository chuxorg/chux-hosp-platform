# Local development setup

1. Clone the private repository and enter its root.
2. Install the version-controlled pre-commit hook: `git config core.hooksPath .githooks`.
3. Before each commit, run `bash scripts/validate.sh`. The hook will also invoke it on commit attempts.
4. Keep GitHub Actions green on the PR. Do not bypass hooks or CI.
5. Refer to `AGENTS.md`, `docs/SDLC.md`, the approved requirements in Dropbox, and the linked story before coding.

**Important:** The bootstrap validator checks only repository policy. There are no application tests yet. When a technology stack is chosen, replace the bootstrap guard with actual lint/type/unit/integration commands and corresponding CI jobs **before** introducing application code.

Git hooks are local conveniences, not security controls: another machine or agent can skip them. GitHub CI provides objective verification, and protected-branch required checks provide enforceable merge gating when available for the organization plan.
