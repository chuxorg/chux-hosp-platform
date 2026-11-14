"""Database wiring and readiness checks, independent of domain models."""

import os

from sqlalchemy import Engine, create_engine, text
from sqlalchemy.exc import SQLAlchemyError

DEFAULT_DATABASE_URL = "sqlite+pysqlite:///./hospitality-local.db"


def make_engine(database_url: str | None = None) -> Engine:
    url = database_url or os.environ.get("DATABASE_URL", DEFAULT_DATABASE_URL)
    return create_engine(url, pool_pre_ping=True)


def database_is_ready(engine: Engine) -> bool:
    try:
        with engine.connect() as connection:
            return connection.execute(text("SELECT 1")).scalar_one() == 1
    except SQLAlchemyError:
        return False
