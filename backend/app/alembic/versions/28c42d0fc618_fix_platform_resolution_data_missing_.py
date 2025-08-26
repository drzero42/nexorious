"""Fix platform_resolution_data missing original_name field

Revision ID: 28c42d0fc618
Revises: 7b8f517ecfe3
Create Date: 2025-08-25 13:03:15.298458

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
import sqlmodel


# revision identifiers, used by Alembic.
revision: str = '28c42d0fc618'
down_revision: Union[str, Sequence[str], None] = '7b8f517ecfe3'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Fix platform_resolution_data missing original_name field in existing records."""
    import json
    
    # Get database connection
    connection = op.get_bind()
    
    # Query all DarkadiaImport records with platform_resolution_data that need fixing
    result = connection.execute(
        sa.text("""
            SELECT id, platform_resolution_data, original_platform_name 
            FROM darkadia_imports 
            WHERE platform_resolution_data IS NOT NULL 
            AND platform_resolution_data != '{}'
            AND JSON_EXTRACT(platform_resolution_data, '$.original_name') IS NULL
        """)
    )
    
    records_updated = 0
    
    for row in result:
        try:
            # Parse existing JSON data
            current_data = json.loads(row.platform_resolution_data)
            
            # Add the missing original_name field from original_platform_name column
            current_data['original_name'] = row.original_platform_name or ""
            
            # Add other missing fields to match PlatformResolutionData schema
            if 'suggestions' not in current_data:
                current_data['suggestions'] = []
            if 'storefront_suggestions' not in current_data:
                current_data['storefront_suggestions'] = []
            if 'resolved_platform_id' not in current_data:
                current_data['resolved_platform_id'] = None
            if 'resolved_storefront_id' not in current_data:
                current_data['resolved_storefront_id'] = None
            if 'resolution_timestamp' not in current_data:
                current_data['resolution_timestamp'] = None
            if 'resolution_method' not in current_data:
                current_data['resolution_method'] = None
            if 'user_notes' not in current_data:
                current_data['user_notes'] = None
            
            # Update the record
            connection.execute(
                sa.text("UPDATE darkadia_imports SET platform_resolution_data = :data WHERE id = :id"),
                {"data": json.dumps(current_data), "id": row.id}
            )
            records_updated += 1
            
        except (json.JSONDecodeError, Exception) as e:
            print(f"Warning: Failed to update record {row.id}: {e}")
            continue
    
    print(f"Updated {records_updated} DarkadiaImport records with missing original_name field")


def downgrade() -> None:
    """Revert the platform_resolution_data changes (remove added fields)."""
    import json
    
    # Get database connection
    connection = op.get_bind()
    
    # Query all DarkadiaImport records with platform_resolution_data
    result = connection.execute(
        sa.text("""
            SELECT id, platform_resolution_data 
            FROM darkadia_imports 
            WHERE platform_resolution_data IS NOT NULL 
            AND platform_resolution_data != '{}'
        """)
    )
    
    records_reverted = 0
    
    for row in result:
        try:
            # Parse existing JSON data
            current_data = json.loads(row.platform_resolution_data)
            
            # Remove the fields we added in upgrade
            fields_to_remove = ['original_name', 'suggestions', 'storefront_suggestions', 
                              'resolved_platform_id', 'resolved_storefront_id', 
                              'resolution_timestamp', 'resolution_method', 'user_notes']
            
            original_fields = {k: v for k, v in current_data.items() if k not in fields_to_remove}
            
            # Only update if we actually removed something
            if len(original_fields) != len(current_data):
                connection.execute(
                    sa.text("UPDATE darkadia_imports SET platform_resolution_data = :data WHERE id = :id"),
                    {"data": json.dumps(original_fields), "id": row.id}
                )
                records_reverted += 1
                
        except (json.JSONDecodeError, Exception) as e:
            print(f"Warning: Failed to revert record {row.id}: {e}")
            continue
    
    print(f"Reverted {records_reverted} DarkadiaImport records")
