"""ASGI entrypoint for uvicorn hospitality.main:app."""

from hospitality.app import create_app

app = create_app()
