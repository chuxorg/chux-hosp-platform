# Hospitality Platform

Greenfield, **modular Hospitality Platform**: independent PMS, Experiences, Loyalty, Procurement, and Operations modules inside one deployable application. **PMS is the first functional delivery**; Thai Cooking demonstrates the separate Experiences framework later. No distributed services, real payment processing or live integrations required initially.

## Project references
- [GitHub Project #14](https://github.com/orgs/chuxorg/projects/14)
- [Dropbox Hospitality knowledge base](https://www.dropbox.com/home/Hospitality)
- [Codex instructions](AGENTS.md)
- [SDLC](docs/SDLC.md)
- [Local setup and tests](docs/LOCAL_SETUP.md)
- [ADR-0001 historical Python decision (superseded)](docs/adr/ADR-0001-python-fastapi-postgres.md)
- [ADR-0003 Go and PostgreSQL replacement decision](docs/adr/ADR-0003-go-postgres-defer-redis.md)
- [ADR-0002 modular platform architecture](docs/adr/ADR-0002-modular-hospitality-platform.md)
- [Approved module boundary map (Dropbox)](https://www.dropbox.com/scl/fi/4ia47cl33y9ibtighkm3j/Hospitality-Modular-Platform-Module-Boundaries-and-Integration-Contracts.paper?rlkey=x5zlc6w9s6u2w9yqh94yye8yk&dl=0)

## Current backend bootstrap (transition pending)
The target stack is **Go + PostgreSQL from day one**, with Docker for local PostgreSQL, a versioned compiled executable, Go tests and migration tooling. Redis is **not** required until a measured use case justifies caching. [Issue #31](https://github.com/chuxorg/chux-hosp-platform/issues/31) tracks replacement of the existing **Python** scaffold. Until that change merges, these are still the commands for the **current code**, not the target stack.

Python 3.13 + FastAPI, SQLAlchemy/Alembic, SQLite local and PostgreSQL deployment target. No PMS or Experiences business workflows implemented yet; development will proceed through independently testable module slices. Readiness endpoints: GET /healthz and GET /readyz.

~~~bash
python3.13 -m venv .venv
source .venv/bin/activate
python -m pip install -e '.[dev]'
bash scripts/validate.sh
alembic upgrade head
uvicorn hospitality.main:app --reload
~~~

**Current** quality workflow still executes Python unit tests, SQLite/PostgreSQL integration, and lint/format checks; it will change in Issue #31. The bootstrap-validation required check aggregates both test jobs. Original PHP, DB and guest data must remain outside Git.
