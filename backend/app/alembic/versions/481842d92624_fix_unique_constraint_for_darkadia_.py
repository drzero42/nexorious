"""Fix unique constraint for darkadia imports to include copy identifier

Revision ID: 481842d92624
Revises: 28c42d0fc618
Create Date: 2025-08-26 07:41:05.000537

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '481842d92624'
down_revision: Union[str, Sequence[str], None] = '28c42d0fc618'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade schema."""
    # Drop the old unique constraint that doesn't include copy_identifier
    with op.batch_alter_table('darkadia_imports', schema=None) as batch_op:
        batch_op.drop_constraint('uq_darkadia_imports_user_row_batch', type_='unique')
        # Create new unique constraint that includes copy_identifier to allow multiple copies per CSV row
        batch_op.create_unique_constraint(
            'uq_darkadia_imports_user_row_copy_batch', 
            ['user_id', 'csv_row_number', 'copy_identifier', 'batch_id']
        )


def downgrade() -> None:
    """Downgrade schema."""
    # Revert to the old constraint
    with op.batch_alter_table('darkadia_imports', schema=None) as batch_op:
        batch_op.drop_constraint('uq_darkadia_imports_user_row_copy_batch', type_='unique')
        batch_op.create_unique_constraint(
            'uq_darkadia_imports_user_row_batch',
            ['user_id', 'csv_row_number', 'batch_id']
        )
