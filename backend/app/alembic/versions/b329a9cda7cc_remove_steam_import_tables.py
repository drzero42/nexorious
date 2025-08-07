"""Remove Steam import tables

Revision ID: b329a9cda7cc
Revises: 33ece1f68c49
Create Date: 2025-08-07 06:25:51.655215

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = 'b329a9cda7cc'
down_revision: Union[str, Sequence[str], None] = '33ece1f68c49'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Drop Steam import tables."""
    # Drop the tables with proper foreign key constraints
    op.drop_index(op.f('ix_steam_import_games_steam_appid'), table_name='steam_import_games')
    op.drop_index(op.f('ix_steam_import_games_status'), table_name='steam_import_games')
    op.drop_index(op.f('ix_steam_import_games_import_job_id'), table_name='steam_import_games')
    op.drop_table('steam_import_games')
    
    op.drop_index(op.f('ix_steam_import_jobs_user_id'), table_name='steam_import_jobs')
    op.drop_index(op.f('ix_steam_import_jobs_status'), table_name='steam_import_jobs')
    op.drop_table('steam_import_jobs')


def downgrade() -> None:
    """Recreate Steam import tables."""
    # Note: This downgrade recreates the tables with minimal schema
    # This is mainly for testing purposes as we don't expect to rollback this removal
    op.create_table('steam_import_jobs',
    sa.Column('id', sa.String(), nullable=False),
    sa.Column('user_id', sa.String(), nullable=False),
    sa.Column('status', sa.String(), nullable=False),
    sa.Column('total_games', sa.Integer(), nullable=False),
    sa.Column('processed_games', sa.Integer(), nullable=False),
    sa.Column('matched_games', sa.Integer(), nullable=False),
    sa.Column('awaiting_review_games', sa.Integer(), nullable=False),
    sa.Column('skipped_games', sa.Integer(), nullable=False),
    sa.Column('imported_games', sa.Integer(), nullable=False),
    sa.Column('platform_added_games', sa.Integer(), nullable=False),
    sa.Column('error_message', sa.String(), nullable=True),
    sa.Column('steam_library_data', sa.String(), nullable=False),
    sa.Column('created_at', sa.DateTime(timezone=True), nullable=False),
    sa.Column('updated_at', sa.DateTime(timezone=True), nullable=False),
    sa.Column('completed_at', sa.DateTime(timezone=True), nullable=True),
    sa.ForeignKeyConstraint(['user_id'], ['users.id'], ),
    sa.PrimaryKeyConstraint('id')
    )
    op.create_index(op.f('ix_steam_import_jobs_status'), 'steam_import_jobs', ['status'], unique=False)
    op.create_index(op.f('ix_steam_import_jobs_user_id'), 'steam_import_jobs', ['user_id'], unique=False)
    
    op.create_table('steam_import_games',
    sa.Column('id', sa.String(), nullable=False),
    sa.Column('import_job_id', sa.String(), nullable=False),
    sa.Column('steam_appid', sa.Integer(), nullable=False),
    sa.Column('steam_name', sa.String(), nullable=False),
    sa.Column('status', sa.String(), nullable=False),
    sa.Column('matched_game_id', sa.String(), nullable=True),
    sa.Column('user_decision', sa.String(), nullable=True),
    sa.Column('error_message', sa.String(), nullable=True),
    sa.Column('created_at', sa.DateTime(timezone=True), nullable=False),
    sa.Column('updated_at', sa.DateTime(timezone=True), nullable=False),
    sa.ForeignKeyConstraint(['import_job_id'], ['steam_import_jobs.id'], ),
    sa.ForeignKeyConstraint(['matched_game_id'], ['games.id'], ),
    sa.PrimaryKeyConstraint('id')
    )
    op.create_index(op.f('ix_steam_import_games_import_job_id'), 'steam_import_games', ['import_job_id'], unique=False)
    op.create_index(op.f('ix_steam_import_games_status'), 'steam_import_games', ['status'], unique=False)
    op.create_index(op.f('ix_steam_import_games_steam_appid'), 'steam_import_games', ['steam_appid'], unique=False)
