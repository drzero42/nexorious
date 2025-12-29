"""
Backup and restore API endpoints (admin-only).
"""

from fastapi import APIRouter, Depends, HTTPException, status, UploadFile, File
from fastapi.responses import FileResponse
from sqlmodel import Session, select
from typing import Annotated
import logging
import tempfile
from pathlib import Path

from ..core.database import get_session
from ..core.security import get_current_admin_user
from ..models.user import User, UserSession
from ..models.backup_config import BackupConfig, BackupSchedule, RetentionMode
from ..schemas.backup import (
    BackupConfigResponse,
    BackupConfigUpdateRequest,
    BackupInfo,
    BackupStats,
    BackupListResponse,
    BackupCreateResponse,
    BackupDeleteResponse,
    RestoreRequest,
    RestoreResponse,
    BackupType as SchemaBackupType,
)
from ..services.backup_service import backup_service, BackupType
from ..worker.tasks.maintenance.backup_create import create_backup_task

router = APIRouter(prefix="/admin/backups", tags=["Backup & Restore (Admin)"])
logger = logging.getLogger(__name__)


def _get_or_create_config(session: Session) -> BackupConfig:
    """Get existing config or create default."""
    config = session.get(BackupConfig, 1)
    if not config:
        config = BackupConfig(id=1)
        session.add(config)
        session.commit()
        session.refresh(config)
    return config


# Configuration endpoints
@router.get("/config", response_model=BackupConfigResponse)
async def get_backup_config(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
):
    """Get backup configuration."""
    config = _get_or_create_config(session)
    return BackupConfigResponse(
        schedule=config.schedule.value,
        schedule_time=config.schedule_time,
        schedule_day=config.schedule_day,
        retention_mode=config.retention_mode.value,
        retention_value=config.retention_value,
        updated_at=config.updated_at,
    )


@router.put("/config", response_model=BackupConfigResponse)
async def update_backup_config(
    config_update: BackupConfigUpdateRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
):
    """Update backup configuration."""
    from datetime import datetime, timezone

    config = _get_or_create_config(session)

    update_data = config_update.model_dump(exclude_unset=True)

    for field, value in update_data.items():
        if field == "schedule" and value:
            setattr(config, field, BackupSchedule(value))
        elif field == "retention_mode" and value:
            setattr(config, field, RetentionMode(value))
        else:
            setattr(config, field, value)

    config.updated_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(config)

    return BackupConfigResponse(
        schedule=config.schedule.value,
        schedule_time=config.schedule_time,
        schedule_day=config.schedule_day,
        retention_mode=config.retention_mode.value,
        retention_value=config.retention_value,
        updated_at=config.updated_at,
    )


# Backup operations
@router.post("", response_model=BackupCreateResponse, status_code=status.HTTP_202_ACCEPTED)
async def create_backup(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
):
    """Create a new backup (manual trigger).

    This dispatches a background task to the worker to create the backup.
    The backup will be available in the list once completed.
    """
    logger.info(f"User {current_user.id} requesting manual backup")

    # Dispatch backup task to worker
    await create_backup_task.kiq(backup_type=BackupType.MANUAL.value)

    return BackupCreateResponse(
        job_id="pending",
        message="Backup job dispatched. Check the backup list for status.",
    )


@router.get("", response_model=BackupListResponse)
async def list_backups(
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
):
    """List all available backups."""
    backups = backup_service.list_backups()

    backup_infos = [
        BackupInfo(
            id=b.id,
            created_at=b.created_at,
            backup_type=SchemaBackupType(b.backup_type.value),
            size_bytes=b.size_bytes,
            stats=BackupStats(
                users=b.stats_users,
                games=b.stats_games,
                tags=b.stats_tags,
            ),
        )
        for b in backups
    ]

    return BackupListResponse(
        backups=backup_infos,
        total=len(backup_infos),
    )


@router.get("/{backup_id}/download")
async def download_backup(
    backup_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
):
    """Download a backup file."""
    backup_path = backup_service.get_backup_path(backup_id)

    if not backup_path.exists():
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Backup not found",
        )

    return FileResponse(
        path=backup_path,
        filename=f"{backup_id}.tar.gz",
        media_type="application/gzip",
    )


@router.delete("/{backup_id}", response_model=BackupDeleteResponse)
async def delete_backup(
    backup_id: str,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
):
    """Delete a backup."""
    if backup_service.delete_backup(backup_id):
        return BackupDeleteResponse(
            success=True,
            message=f"Backup deleted: {backup_id}",
        )
    else:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Backup not found",
        )


# Restore operations
@router.post("/{backup_id}/restore", response_model=RestoreResponse)
async def restore_backup(
    backup_id: str,
    restore_request: RestoreRequest,
    session: Annotated[Session, Depends(get_session)],
    current_user: Annotated[User, Depends(get_current_admin_user)],
):
    """Restore from a server backup."""
    if not restore_request.confirm:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Restore must be confirmed by setting confirm=true",
        )

    # Get admin's current session data before restore
    admin_session = session.exec(
        select(UserSession).where(UserSession.user_id == current_user.id)
    ).first()

    session_data = None
    if admin_session:
        session_data = {
            "id": admin_session.id,
            "token_hash": admin_session.token_hash,
            "refresh_token_hash": admin_session.refresh_token_hash,
            "expires_at": admin_session.expires_at,
            "ip_address": admin_session.ip_address,
            "user_agent": admin_session.user_agent,
        }

    try:
        backup_service.restore_backup(
            backup_id=backup_id,
            admin_user_id=current_user.id,
            admin_session_data=session_data,
        )

        return RestoreResponse(
            success=True,
            message=f"Restore completed from: {backup_id}",
            session_invalidated=False,
        )
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        )
    except RuntimeError as e:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        )


@router.post("/restore/upload", response_model=RestoreResponse)
async def restore_from_upload(
    restore_request: RestoreRequest,
    file: UploadFile = File(...),
    session: Session = Depends(get_session),
    current_user: User = Depends(get_current_admin_user),
):
    """Restore from an uploaded backup file."""
    if not restore_request.confirm:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Restore must be confirmed by setting confirm=true",
        )

    if not file.filename or not file.filename.endswith(".tar.gz"):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Invalid file format. Expected .tar.gz",
        )

    # Save uploaded file to temp location
    with tempfile.NamedTemporaryFile(delete=False, suffix=".tar.gz") as tmp:
        content = await file.read()
        tmp.write(content)
        tmp_path = Path(tmp.name)

    try:
        # Validate with checksum verification for uploads
        backup_service.validate_backup_archive(tmp_path, verify_checksums=True)

        # Get admin's current session data
        admin_session = session.exec(
            select(UserSession).where(UserSession.user_id == current_user.id)
        ).first()

        session_data = None
        if admin_session:
            session_data = {
                "id": admin_session.id,
                "token_hash": admin_session.token_hash,
                "refresh_token_hash": admin_session.refresh_token_hash,
                "expires_at": admin_session.expires_at,
                "ip_address": admin_session.ip_address,
                "user_agent": admin_session.user_agent,
            }

        # Move to backups dir with generated ID
        backup_id = backup_service.generate_backup_id()
        dest_path = backup_service.get_backup_path(backup_id)
        tmp_path.rename(dest_path)

        # Restore
        backup_service.restore_backup(
            backup_id=backup_id,
            admin_user_id=current_user.id,
            admin_session_data=session_data,
        )

        return RestoreResponse(
            success=True,
            message="Restore completed from uploaded backup",
            session_invalidated=False,
        )
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e),
        )
    except RuntimeError as e:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e),
        )
    finally:
        # Clean up temp file if it still exists
        if tmp_path.exists():
            tmp_path.unlink()
