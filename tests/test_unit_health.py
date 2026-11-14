"""Unit tests for API behavior and database readiness failure handling."""

from unittest.mock import MagicMock

from fastapi.testclient import TestClient
from sqlalchemy.exc import OperationalError

from hospitality.app import create_app
from hospitality.db import database_is_ready


def test_live_health_does_not_require_database() -> None:
    app = create_app("sqlite+pysqlite:///:memory:")
    with TestClient(app) as client:
        response = client.get("/healthz")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}
    app.state.engine.dispose()


def test_database_readiness_failure_is_handled() -> None:
    engine = MagicMock()
    engine.connect.side_effect = OperationalError("SELECT 1", {}, RuntimeError("offline"))
    assert database_is_ready(engine) is False


def test_readiness_endpoint_reports_503(monkeypatch) -> None:
    monkeypatch.setattr("hospitality.app.db.database_is_ready", lambda _engine: False)
    app = create_app("sqlite+pysqlite:///:memory:")
    with TestClient(app) as client:
        response = client.get("/readyz")
    assert response.status_code == 503
    assert response.json() == {"detail": "database unavailable"}
    app.state.engine.dispose()
