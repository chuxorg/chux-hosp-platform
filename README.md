# Hospitality Platform

Greenfield Hospitality platform: foundational PMS/CRM capabilities and an extensible Experience layer, beginning with a small Thailand cooking experience pilot.

## Project links

- [GitHub Project #14](https://github.com/orgs/chuxorg/projects/14/)
- [Dropbox Hospitality knowledge base](https://www.dropbox.com/home/Hospitality)
- [Legacy PMS analysis](https://www.dropbox.com/home/Hospitality/07_Legacy_PMS)
- [Development operating model](docs/SDLC.md) — added in the bootstrap PR

## Boundaries

The legacy PHP application and database are **reference material only**, not source dependencies. Do not commit legacy archives, raw database dumps, personal data, secrets, or credentials. The implementation will use a new technology stack, selected by an explicit architecture decision.

Business knowledge and approved requirements live in Dropbox. GitHub Issues/Projects track delivery. This repository tracks implementation, tests, build tooling, and the instructions needed to execute it.

> Bootstrap note: The initial README establishes `master`. Repository conventions, issue templates, CI, and Codex instructions are proposed via the first pull request.
