"""add user_import_mappings table

Revision ID: e444b5a028ad
Revises: db3b6ae1746c
Create Date: 2025-12-20 13:34:06.911658

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
import sqlmodel

# revision identifiers, used by Alembic.
revision: str = 'e444b5a028ad'
down_revision: Union[str, Sequence[str], None] = 'db3b6ae1746c'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    op.create_table('user_import_mappings',
    sa.Column('id', sqlmodel.sql.sqltypes.AutoString(), nullable=False),
    sa.Column('user_id', sqlmodel.sql.sqltypes.AutoString(), nullable=False),
    sa.Column('import_source', sqlmodel.sql.sqltypes.AutoString(length=50), nullable=False),
    sa.Column('mapping_type', sa.Enum('PLATFORM', 'STOREFRONT', name='importmappingtype'), nullable=False),
    sa.Column('source_value', sqlmodel.sql.sqltypes.AutoString(length=255), nullable=False),
    sa.Column('target_id', sqlmodel.sql.sqltypes.AutoString(length=100), nullable=False),
    sa.Column('created_at', sa.DateTime(), nullable=False),
    sa.Column('updated_at', sa.DateTime(), nullable=False),
    sa.ForeignKeyConstraint(['user_id'], ['users.id'], ),
    sa.PrimaryKeyConstraint('id'),
    sa.UniqueConstraint('user_id', 'import_source', 'mapping_type', 'source_value', name='uq_user_import_mapping')
    )
    op.create_index(op.f('ix_user_import_mappings_import_source'), 'user_import_mappings', ['import_source'], unique=False)
    op.create_index(op.f('ix_user_import_mappings_mapping_type'), 'user_import_mappings', ['mapping_type'], unique=False)
    op.create_index(op.f('ix_user_import_mappings_user_id'), 'user_import_mappings', ['user_id'], unique=False)


def downgrade() -> None:
    """Downgrade schema."""
    op.drop_index(op.f('ix_user_import_mappings_user_id'), table_name='user_import_mappings')
    op.drop_index(op.f('ix_user_import_mappings_mapping_type'), table_name='user_import_mappings')
    op.drop_index(op.f('ix_user_import_mappings_import_source'), table_name='user_import_mappings')
    op.drop_table('user_import_mappings')
    op.execute("DROP TYPE IF EXISTS importmappingtype")
