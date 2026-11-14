"""Minimal platform metadata table for migration and persistence verification.

Revision ID: 0001_platform_metadata
Revises:
"""

import sqlalchemy as sa
from alembic import op

revision = "0001_platform_metadata"
down_revision = None
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "platform_metadata",
        sa.Column("key", sa.String(length=80), primary_key=True),
        sa.Column("value", sa.String(length=250), nullable=False),
    )


def downgrade() -> None:
    op.drop_table("platform_metadata")
