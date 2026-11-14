# Local developer setup

Use Python 3.13 and Git. Keep the legacy PHP/MyISAM archives and guest data outside this repository.

~~~bash
python3.13 -m venv .venv
source .venv/bin/activate
python -m pip install -e '.[dev]'
git config core.hooksPath .githooks
bash scripts/validate.sh
~~~

The validator runs Python compilation, pytest unit and real SQLite integration tests, Ruff lint and Ruff formatting. The pre-commit hook invokes the validator; install it per clone.

To run the sample API with SQLite:

~~~bash
alembic upgrade head
uvicorn hospitality.main:app --reload
# GET http://127.0.0.1:8000/healthz and /readyz
~~~

Optional local PostgreSQL parity (Docker required):

~~~bash
docker compose up -d postgres
export DATABASE_URL='postgresql+psycopg://hospitality:local_only_change_me@localhost:5432/hospitality'
alembic upgrade head
uvicorn hospitality.main:app --reload
# Create a separate database for destructive migration tests:
docker compose exec postgres createdb -U hospitality hospitality_test
TEST_DATABASE_URL='postgresql+psycopg://hospitality:local_only_change_me@localhost:5432/hospitality_test' python -m pytest -q -m integration
~~~

These credentials are localhost-only development examples. Never deploy them publicly or reuse them in production. CI starts a fresh isolated PostgreSQL service. Application unit/integration tests must grow with each approved story.
