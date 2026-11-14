# Hospitality Platform

Greenfield Hospitality platform with foundational PMS/CRM capabilities and an extensible Experience layer. Phase 1 is a small Thailand cooking pilot with simulated payments/commissions and no live integrations.

## Project references
- [GitHub Project #14](https://github.com/orgs/chuxorg/projects/14)
- [Dropbox Hospitality knowledge base](https://www.dropbox.com/home/Hospitality)
- [Codex instructions](AGENTS.md)
- [SDLC](docs/SDLC.md)
- [Local setup and tests](docs/LOCAL_SETUP.md)
- [ADR-0001 accepted backend decision](docs/adr/ADR-0001-python-fastapi-postgres.md)

## Backend bootstrap
Python 3.13 + FastAPI, SQLAlchemy/Alembic, SQLite local and PostgreSQL deployment target. No PMS or Experience workflows implemented yet. Readiness endpoints: GET /healthz and GET /readyz.

~~~bash
python3.13 -m venv .venv
source .venv/bin/activate
python -m pip install -e '.[dev]'
bash scripts/validate.sh
alembic upgrade head
uvicorn hospitality.main:app --reload
~~~

The quality workflow executes unit tests, SQLite integration, PostgreSQL integration, and lint/format checks. The bootstrap-validation required check aggregates both test jobs. Original PHP, DB and guest data must remain outside Git.
