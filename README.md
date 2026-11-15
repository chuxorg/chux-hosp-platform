# Hospitality Platform

Greenfield, **modular Hospitality Platform**: independent PMS, Experiences, Loyalty, Procurement, and Operations modules inside one FastAPI application. **PMS is the first functional delivery**; Thai Cooking demonstrates the separate Experiences framework later. No distributed services, real payment processing or live integrations required initially.

## Project references
- [GitHub Project #14](https://github.com/orgs/chuxorg/projects/14)
- [Dropbox Hospitality knowledge base](https://www.dropbox.com/home/Hospitality)
- [Codex instructions](AGENTS.md)
- [SDLC](docs/SDLC.md)
- [Local setup and tests](docs/LOCAL_SETUP.md)
- [ADR-0001 accepted backend decision](docs/adr/ADR-0001-python-fastapi-postgres.md)
- [ADR-0002 modular platform architecture](docs/adr/ADR-0002-modular-hospitality-platform.md)
- [Approved module boundary map (Dropbox)](https://www.dropbox.com/scl/fi/4ia47cl33y9ibtighkm3j/Hospitality-Modular-Platform-Module-Boundaries-and-Integration-Contracts.paper?rlkey=x5zlc6w9s6u2w9yqh94yye8yk&dl=0)

## Backend bootstrap
Python 3.13 + FastAPI, SQLAlchemy/Alembic, SQLite local and PostgreSQL deployment target. No PMS or Experiences business workflows implemented yet; development will proceed through independently testable module slices. Readiness endpoints: GET /healthz and GET /readyz.

~~~bash
python3.13 -m venv .venv
source .venv/bin/activate
python -m pip install -e '.[dev]'
bash scripts/validate.sh
alembic upgrade head
uvicorn hospitality.main:app --reload
~~~

The quality workflow executes unit tests, SQLite integration, PostgreSQL integration, and lint/format checks. The bootstrap-validation required check aggregates both test jobs. Original PHP, DB and guest data must remain outside Git.
