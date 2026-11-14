# ADR-0001: Python/FastAPI backend with portable relational persistence

Status: Accepted — approved by project owner on 2026-11-14  
Date: 2026-11-14  
Owner: Hospitality project owner  
Refs: Issue #3; REQ-PLT-001, REQ-PLT-002; Dropbox greenfield specification

## Context
We need a small greenfield PMS/CRM foundation and independent Experience layer, starting with a Thailand cooking pilot. Legacy PHP/MyISAM is evidence only. Initial development should work without remote services, be testable in VS Code/Codex, and move to a production relational database without redesigning the domain.

## Options considered
1. TypeScript/Node/Fastify with a typed SQL layer: good full-stack language option but more up-front tooling for the current backend-oriented discovery phase.
2. Python/Django: mature ORM/admin, but more framework footprint than the pilot requires.
3. Python/FastAPI + SQLAlchemy/Alembic: small typed API, test support, framework-independent domain layer, mature migrations and SQLite/PostgreSQL drivers.

## Accepted decision
- Backend: Python 3.13, FastAPI, Pydantic, SQLAlchemy 2.x and Alembic; sync database access initially.
- Database: SQLite for local isolated tests and optional local pilot, PostgreSQL for deployment; containerized PostgreSQL in CI and for local parity.
- Package management: pyproject.toml, isolated venv; add a full transitive dependency lock before production.
- Tests: pytest + FastAPI TestClient; actual SQLAlchemy/Alembic migrations and persistence against SQLite and PostgreSQL; Ruff for lint/format.
- Deployment: modular monolith, clear domain boundaries; no microservices, real payments or external integrations in Pilot.
- Frontend: not selected here; choose in a follow-on ADR when user-facing stories are approved.

## Tradeoffs
Dual SQLite/PostgreSQL support must be continuously tested against PostgreSQL. A future TypeScript web UI would introduce a second language. The bootstrap API has only liveness/readiness plus platform metadata — not booking functionality. Dependency ranges are bounded but not transitively locked; lock before deployment.

## Verification
A clean checkout runs unit and SQLite integration via bash scripts/validate.sh. CI repeats real PostgreSQL migration and read/write integration, and the required bootstrap-validation job aggregates both test jobs. Approved product-domain stories add feature acceptance tests.
