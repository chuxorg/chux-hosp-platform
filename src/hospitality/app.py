"""Minimal API bootstrap; intentionally contains no product workflow yet."""

from fastapi import FastAPI, HTTPException

from hospitality import db


def create_app(database_url: str | None = None) -> FastAPI:
    app = FastAPI(title="Hospitality Platform", version="0.1.0")
    engine = db.make_engine(database_url)
    app.state.engine = engine

    @app.get("/healthz")
    def healthz() -> dict[str, str]:
        return {"status": "ok"}

    @app.get("/readyz")
    def readyz() -> dict[str, str]:
        if not db.database_is_ready(engine):
            raise HTTPException(status_code=503, detail="database unavailable")
        return {"status": "ready"}

    return app
