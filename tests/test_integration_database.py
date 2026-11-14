"""Real API-to-SQLAlchemy connectivity and Alembic persistence tests.

Uses isolated SQLite in local runs; GitHub Actions reruns against a PostgreSQL service.
"""

import os

import pytest
from alembic import command
from alembic.config import Config
from fastapi.testclient import TestClient
from sqlalchemy import text

from hospitality.app import create_app
from hospitality.db import make_engine


@pytest.fixture
def migrated_db(tmp_path, monkeypatch):
    database_url = os.environ.get("TEST_DATABASE_URL") or (
        f"sqlite+pysqlite:///{tmp_path / 'integration.db'}"
    )
    monkeypatch.setenv("DATABASE_URL", database_url)
    config = Config("alembic.ini")
    command.upgrade(config, "head")
    try:
        yield database_url
    finally:
        command.downgrade(config, "base")


@pytest.mark.integration
def test_ready_endpoint_reads_real_database(migrated_db: str) -> None:
    app = create_app(migrated_db)
    with TestClient(app) as client:
        response = client.get("/readyz")
    assert response.status_code == 200
    assert response.json() == {"status": "ready"}
    app.state.engine.dispose()


@pytest.mark.integration
def test_schema_migration_allows_parameterized_round_trip(migrated_db: str) -> None:
    engine = make_engine(migrated_db)
    try:
        with engine.begin() as connection:
            connection.execute(
                text("INSERT INTO platform_metadata (key, value) VALUES (:key, :value)"),
                {"key": "pilot_test", "value": "ready"},
            )
            result = connection.execute(
                text("SELECT value FROM platform_metadata WHERE key = :key"),
                {"key": "pilot_test"},
            ).scalar_one()
        assert result == "ready"
    finally:
        engine.dispose()
