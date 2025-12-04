"""
Generic batch processing module for import sources.

This module provides a generic batch processor that can be configured
for different import sources (Steam, Darkadia, etc.), eliminating code
duplication between source-specific batch endpoint files.
"""

from dataclasses import dataclass, field
from typing import (
    Annotated,
    Any,
    Callable,
    Generic,
    List,
    Optional,
    Protocol,
    Sequence,
    Type,
    TypeVar,
    runtime_checkable,
)

from fastapi import APIRouter, Depends, HTTPException, status
from pydantic import BaseModel
from sqlmodel import Session, and_, col, select
import logging

from ...core.database import get_session
from ...models.batch_session import (
    BATCH_SIZES,
    BatchOperationType,
    BatchSessionStatus,
)
from ...services.batch_session_manager import get_batch_session_manager
from ...schemas.batch import (
    BatchCancelResponse,
    BatchNextRequest,
    BatchNextResponse,
    BatchSessionStartRequest,
    BatchSessionStartResponse,
    BatchStatusResponse,
)
from ...utils.sqlalchemy_typed import in_, is_, is_not

logger = logging.getLogger(__name__)

# Type variables for generic game model and response schema
GameModelT = TypeVar("GameModelT")
ResponseSchemaT = TypeVar("ResponseSchemaT", bound=BaseModel)


@runtime_checkable
class ImportGameModel(Protocol):
    """Protocol defining required attributes for import game models."""

    id: str
    user_id: str
    igdb_id: Optional[int]
    game_id: Optional[int]
    game_name: str
    ignored: bool


class AutoMatchResult(Protocol):
    """Protocol for auto-match result from import services."""

    matched: bool
    igdb_id: Optional[int]
    igdb_title: Optional[str]
    error_message: Optional[str]


class SyncResult(Protocol):
    """Protocol for sync result from import services."""

    action: str
    user_game_id: Optional[str]
    error_message: Optional[str]


@runtime_checkable
class ImportSourceService(Protocol):
    """Protocol defining required methods for import services used in batch processing."""

    async def auto_match_game(
        self, user_id: str, game_id: str
    ) -> AutoMatchResult: ...

    async def sync_game(self, user_id: str, game_id: str) -> SyncResult: ...


@dataclass
class BatchSourceConfig(Generic[GameModelT, ResponseSchemaT]):
    """Configuration for a batch processing source.

    This configuration object defines all source-specific details needed
    to create batch processing endpoints for an import source.
    """

    # Source identification
    source_name: str  # e.g., "Steam", "Darkadia"
    router_prefix: str  # e.g., "/batch"
    router_tags: Sequence[str]  # e.g., ["Steam Batch Import"]

    # Model and schema types
    game_model: Type[GameModelT]
    response_schema: Type[ResponseSchemaT]

    # Service factory - function that creates the import service from a session
    service_factory: Callable[[Session], ImportSourceService]

    # Auth dependency - FastAPI dependency for user authentication
    auth_dependency: Any

    # Response field mapper - converts game model to response schema dict
    response_mapper: Callable[[GameModelT, dict], ResponseSchemaT]

    # Optional: custom query conditions builder for filtering games
    # Takes (user_id, processed_item_ids) and returns list of additional conditions
    extra_query_conditions: Optional[
        Callable[[str, List[str]], List[Any]]
    ] = field(default=None)


def create_batch_router(config: BatchSourceConfig) -> APIRouter:
    """Create a configured batch processing router for an import source.

    This factory function creates a complete FastAPI router with all batch
    processing endpoints configured for the specified import source.

    Args:
        config: BatchSourceConfig with all source-specific settings

    Returns:
        Configured APIRouter with batch endpoints
    """
    router = APIRouter(prefix=config.router_prefix, tags=list(config.router_tags))  # type: ignore[arg-type]

    def get_service(session: Annotated[Session, Depends(get_session)]):
        """Dependency to get the import service."""
        return config.service_factory(session)

    # Auto-match batch endpoints
    @router.post(
        "/auto-match/start",
        response_model=BatchSessionStartResponse,
        status_code=status.HTTP_201_CREATED,
    )
    async def start_batch_auto_match(
        _request: BatchSessionStartRequest,
        db_session: Annotated[Session, Depends(get_session)],
        current_user: Annotated[Any, Depends(config.auth_dependency)],
    ) -> BatchSessionStartResponse:
        """Start a new batch auto-matching session."""
        try:
            logger.info(
                f"Starting batch auto-match session for user {current_user.id} "
                f"({config.source_name})"
            )

            # Build query for unmatched games
            query_conditions = [
                config.game_model.user_id == current_user.id,
                is_(col(config.game_model.igdb_id), None),
                is_(config.game_model.ignored, False),
            ]

            if config.extra_query_conditions:
                query_conditions.extend(
                    config.extra_query_conditions(current_user.id, [])
                )

            unmatched_games_query = select(config.game_model).where(
                and_(*query_conditions)
            )
            unmatched_games = db_session.exec(unmatched_games_query).all()
            total_items = len(unmatched_games)

            if total_items == 0:
                logger.info(
                    f"No unmatched {config.source_name} games found for user "
                    f"{current_user.id}"
                )
                return BatchSessionStartResponse(
                    session_id="",
                    total_items=0,
                    operation_type=BatchOperationType.AUTO_MATCH.value,
                    status="completed",
                    message=f"No unmatched {config.source_name} games found to process",
                )

            # Create the batch session
            session_manager = get_batch_session_manager()
            batch_session = session_manager.create_session(
                user_id=current_user.id,
                operation_type=BatchOperationType.AUTO_MATCH,
                total_items=total_items,
            )

            logger.info(
                f"Created batch auto-match session {batch_session.id} for user "
                f"{current_user.id} with {total_items} unmatched games "
                f"({config.source_name})"
            )

            return BatchSessionStartResponse(
                session_id=batch_session.id,
                total_items=total_items,
                operation_type=BatchOperationType.AUTO_MATCH.value,
                status=batch_session.status.value,
                message=f"Batch auto-match session started for {total_items} "
                f"unmatched games",
            )

        except Exception as e:
            logger.error(
                f"Error starting batch auto-match session for user "
                f"{current_user.id} ({config.source_name}): {str(e)}"
            )
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Failed to start batch auto-match session",
            )

    @router.post(
        "/auto-match/{session_id}/next",
        response_model=BatchNextResponse,
        status_code=status.HTTP_200_OK,
    )
    async def process_next_auto_match_batch(
        session_id: str,
        _request: BatchNextRequest,
        db_session: Annotated[Session, Depends(get_session)],
        current_user: Annotated[Any, Depends(config.auth_dependency)],
        service=Depends(get_service),
    ) -> BatchNextResponse:
        """Process the next batch of unmatched games for auto-matching."""
        try:
            logger.info(
                f"Processing next auto-match batch for session {session_id} "
                f"by user {current_user.id} ({config.source_name})"
            )

            # Get the batch session
            session_manager = get_batch_session_manager()
            batch_session = session_manager.get_session(session_id)

            if not batch_session:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail="Batch session not found",
                )

            if batch_session.user_id != current_user.id:
                raise HTTPException(
                    status_code=status.HTTP_403_FORBIDDEN,
                    detail="Access denied to this batch session",
                )

            if not batch_session.is_active:
                logger.warning(
                    f"Attempt to process inactive session {session_id} "
                    f"(status: {batch_session.status})"
                )
                return _create_batch_response(config, batch_session, [], "Session is not active")

            # Get next batch of unmatched games
            batch_size = BATCH_SIZES[BatchOperationType.AUTO_MATCH]
            query_conditions = [
                config.game_model.user_id == current_user.id,
                is_(col(config.game_model.igdb_id), None),
                is_(config.game_model.ignored, False),
            ]

            # Exclude already processed games
            if batch_session.processed_item_ids:
                query_conditions.append(
                    ~in_(
                        col(config.game_model.id),
                        batch_session.processed_item_ids,
                    )
                )

            if config.extra_query_conditions:
                query_conditions.extend(
                    config.extra_query_conditions(
                        current_user.id, batch_session.processed_item_ids
                    )
                )

            unmatched_games_query = (
                select(config.game_model)
                .where(and_(*query_conditions))
                .limit(batch_size)
            )

            games_to_process = db_session.exec(unmatched_games_query).all()

            if not games_to_process:
                # No more games to process - mark session as complete
                if batch_session.status.value != "cancelled":
                    batch_session.status = BatchSessionStatus.COMPLETED
                logger.info(
                    f"Batch auto-match session {session_id} completed - "
                    f"no more games to process ({config.source_name})"
                )
                return _create_batch_response(
                    config, batch_session, [], "No more games to process"
                )

            # Process the batch
            game_ids = [game.id for game in games_to_process]
            game_name_map = {game.id: game.game_name for game in games_to_process}

            logger.info(
                f"Auto-matching {len(game_ids)} games in batch for session "
                f"{session_id} ({config.source_name})"
            )

            successful_count = 0
            failed_count = 0
            errors = []
            match_results = {}
            failed_game_ids = []

            for game_id in game_ids:
                try:
                    result = await service.auto_match_game(
                        current_user.id, game_id
                    )
                    if result.matched:
                        successful_count += 1
                        match_results[game_id] = {
                            "igdb_id": result.igdb_id,
                            "igdb_title": result.igdb_title,
                            "matched": True,
                        }
                    else:
                        failed_count += 1
                        failed_game_ids.append(game_id)
                        error_msg = (
                            result.error_message or "Failed to match game"
                        )
                        errors.append(
                            f'Game "{game_name_map[game_id]}": {error_msg}'
                        )
                        match_results[game_id] = {
                            "matched": False,
                            "error": error_msg,
                        }
                except Exception as e:
                    failed_count += 1
                    failed_game_ids.append(game_id)
                    error_msg = (
                        f'Error auto-matching game '
                        f'"{game_name_map[game_id]}": {str(e)}'
                    )
                    logger.error(error_msg)
                    errors.append(error_msg)
                    match_results[game_id] = {"matched": False, "error": str(e)}

            # Update session progress
            session_manager.update_session_progress(
                session_id=session_id,
                processed_count=len(games_to_process),
                successful_count=successful_count,
                failed_count=failed_count,
                processed_ids=game_ids,
                failed_ids=failed_game_ids,
                errors=errors[-10:],
            )

            # Refresh games from database
            updated_games = db_session.exec(
                select(config.game_model).where(
                    in_(config.game_model.id, game_ids)
                )
            ).all()

            # Convert to response format
            current_batch_items = [
                config.response_mapper(game, match_results.get(game.id, {}))
                for game in updated_games
            ]

            message = (
                f"Processed batch of {len(games_to_process)} games: "
                f"{successful_count} matched, {failed_count} failed"
            )

            logger.info(
                f"Completed auto-match batch processing for session "
                f"{session_id}: {message} ({config.source_name})"
            )

            return BatchNextResponse(
                session_id=batch_session.id,
                batch_processed=len(games_to_process),
                batch_successful=successful_count,
                batch_failed=failed_count,
                batch_errors=errors,
                current_batch_items=current_batch_items,
                total_items=batch_session.total_items,
                processed_items=batch_session.processed_items,
                successful_items=batch_session.successful_items,
                failed_items=batch_session.failed_items,
                remaining_items=batch_session.remaining_items,
                progress_percentage=batch_session.progress_percentage,
                status=batch_session.status.value,
                is_complete=batch_session.is_complete,
                message=message,
            )

        except HTTPException:
            raise
        except Exception as e:
            logger.error(
                f"Error processing auto-match batch for session {session_id} "
                f"({config.source_name}): {str(e)}"
            )

            session_manager = get_batch_session_manager()
            session_manager.fail_session(session_id, str(e))

            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Failed to process batch",
            )

    # Sync batch endpoints
    @router.post(
        "/sync/start",
        response_model=BatchSessionStartResponse,
        status_code=status.HTTP_201_CREATED,
    )
    async def start_batch_sync(
        _request: BatchSessionStartRequest,
        db_session: Annotated[Session, Depends(get_session)],
        current_user: Annotated[Any, Depends(config.auth_dependency)],
    ) -> BatchSessionStartResponse:
        """Start a new batch sync session."""
        try:
            logger.info(
                f"Starting batch sync session for user {current_user.id} "
                f"({config.source_name})"
            )

            # Build query for matched but not synced games
            query_conditions = [
                config.game_model.user_id == current_user.id,
                is_not(col(config.game_model.igdb_id), None),
                is_(col(config.game_model.game_id), None),
                is_(config.game_model.ignored, False),
            ]

            if config.extra_query_conditions:
                query_conditions.extend(
                    config.extra_query_conditions(current_user.id, [])
                )

            matched_games_query = select(config.game_model).where(
                and_(*query_conditions)
            )
            matched_games = db_session.exec(matched_games_query).all()
            total_items = len(matched_games)

            if total_items == 0:
                logger.info(
                    f"No matched {config.source_name} games found to sync "
                    f"for user {current_user.id}"
                )
                return BatchSessionStartResponse(
                    session_id="",
                    total_items=0,
                    operation_type=BatchOperationType.SYNC.value,
                    status="completed",
                    message=f"No matched {config.source_name} games found to sync",
                )

            # Create the batch session
            session_manager = get_batch_session_manager()
            batch_session = session_manager.create_session(
                user_id=current_user.id,
                operation_type=BatchOperationType.SYNC,
                total_items=total_items,
            )

            logger.info(
                f"Created batch sync session {batch_session.id} for user "
                f"{current_user.id} with {total_items} matched games "
                f"({config.source_name})"
            )

            return BatchSessionStartResponse(
                session_id=batch_session.id,
                total_items=total_items,
                operation_type=BatchOperationType.SYNC.value,
                status=batch_session.status.value,
                message=f"Batch sync session started for {total_items} matched games",
            )

        except Exception as e:
            logger.error(
                f"Error starting batch sync session for user {current_user.id} "
                f"({config.source_name}): {str(e)}"
            )
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Failed to start batch sync session",
            )

    @router.post(
        "/sync/{session_id}/next",
        response_model=BatchNextResponse,
        status_code=status.HTTP_200_OK,
    )
    async def process_next_sync_batch(
        session_id: str,
        _request: BatchNextRequest,
        db_session: Annotated[Session, Depends(get_session)],
        current_user: Annotated[Any, Depends(config.auth_dependency)],
        service=Depends(get_service),
    ) -> BatchNextResponse:
        """Process the next batch of matched games for syncing to collection."""
        try:
            logger.info(
                f"Processing next sync batch for session {session_id} "
                f"by user {current_user.id} ({config.source_name})"
            )

            # Get the batch session
            session_manager = get_batch_session_manager()
            batch_session = session_manager.get_session(session_id)

            if not batch_session:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail="Batch session not found",
                )

            if batch_session.user_id != current_user.id:
                raise HTTPException(
                    status_code=status.HTTP_403_FORBIDDEN,
                    detail="Access denied to this batch session",
                )

            if not batch_session.is_active:
                logger.warning(
                    f"Attempt to process inactive session {session_id} "
                    f"(status: {batch_session.status})"
                )
                return _create_batch_response(config, batch_session, [], "Session is not active")

            # Get next batch of matched games to sync
            batch_size = BATCH_SIZES[BatchOperationType.SYNC]
            query_conditions = [
                config.game_model.user_id == current_user.id,
                is_not(col(config.game_model.igdb_id), None),
                is_(col(config.game_model.game_id), None),
                is_(config.game_model.ignored, False),
            ]

            # Exclude already processed games
            if batch_session.processed_item_ids:
                query_conditions.append(
                    ~in_(
                        col(config.game_model.id),
                        batch_session.processed_item_ids,
                    )
                )

            if config.extra_query_conditions:
                query_conditions.extend(
                    config.extra_query_conditions(
                        current_user.id, batch_session.processed_item_ids
                    )
                )

            matched_games_query = (
                select(config.game_model)
                .where(and_(*query_conditions))
                .limit(batch_size)
            )

            games_to_process = db_session.exec(matched_games_query).all()

            if not games_to_process:
                # No more games to process - mark session as complete
                if batch_session.status.value != "cancelled":
                    batch_session.status = BatchSessionStatus.COMPLETED
                logger.info(
                    f"Batch sync session {session_id} completed - "
                    f"no more games to process ({config.source_name})"
                )
                return _create_batch_response(
                    config, batch_session, [], "No more games to process"
                )

            # Process the batch
            game_ids = [game.id for game in games_to_process]
            game_name_map = {game.id: game.game_name for game in games_to_process}

            logger.info(
                f"Syncing {len(game_ids)} games in batch for session "
                f"{session_id} ({config.source_name})"
            )

            successful_count = 0
            failed_count = 0
            errors = []
            sync_results = {}
            failed_game_ids = []

            for game_id in game_ids:
                try:
                    result = await service.sync_game(current_user.id, game_id)
                    if result.action in ["created", "updated"]:
                        successful_count += 1
                        sync_results[game_id] = {
                            "user_game_id": result.user_game_id,
                            "action": result.action,
                        }
                    else:
                        failed_count += 1
                        failed_game_ids.append(game_id)
                        error_msg = (
                            result.error_message or "Failed to sync game"
                        )
                        errors.append(
                            f'Game "{game_name_map[game_id]}": {error_msg}'
                        )
                except Exception as e:
                    failed_count += 1
                    failed_game_ids.append(game_id)
                    error_msg = (
                        f'Error syncing game "{game_name_map[game_id]}": {str(e)}'
                    )
                    logger.error(error_msg)
                    errors.append(error_msg)

            # Update session progress
            session_manager.update_session_progress(
                session_id=session_id,
                processed_count=len(games_to_process),
                successful_count=successful_count,
                failed_count=failed_count,
                processed_ids=game_ids,
                failed_ids=failed_game_ids,
                errors=errors[-10:],
            )

            # Refresh games from database
            updated_games = db_session.exec(
                select(config.game_model).where(
                    in_(config.game_model.id, game_ids)
                )
            ).all()

            # Convert to response format
            current_batch_items = [
                config.response_mapper(game, sync_results.get(game.id, {}))
                for game in updated_games
            ]

            message = (
                f"Processed batch of {len(games_to_process)} games: "
                f"{successful_count} synced, {failed_count} failed"
            )

            logger.info(
                f"Completed sync batch processing for session {session_id}: "
                f"{message} ({config.source_name})"
            )

            return BatchNextResponse(
                session_id=batch_session.id,
                batch_processed=len(games_to_process),
                batch_successful=successful_count,
                batch_failed=failed_count,
                batch_errors=errors,
                current_batch_items=current_batch_items,
                total_items=batch_session.total_items,
                processed_items=batch_session.processed_items,
                successful_items=batch_session.successful_items,
                failed_items=batch_session.failed_items,
                remaining_items=batch_session.remaining_items,
                progress_percentage=batch_session.progress_percentage,
                status=batch_session.status.value,
                is_complete=batch_session.is_complete,
                message=message,
            )

        except HTTPException:
            raise
        except Exception as e:
            logger.error(
                f"Error processing sync batch for session {session_id} "
                f"({config.source_name}): {str(e)}"
            )

            session_manager = get_batch_session_manager()
            session_manager.fail_session(session_id, str(e))

            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Failed to process batch",
            )

    # Common batch session endpoints
    @router.get(
        "/{session_id}/status",
        response_model=BatchStatusResponse,
        status_code=status.HTTP_200_OK,
    )
    async def get_batch_status(
        session_id: str,
        current_user: Annotated[Any, Depends(config.auth_dependency)],
    ) -> BatchStatusResponse:
        """Get the current status of a batch session."""
        try:
            session_manager = get_batch_session_manager()
            batch_session = session_manager.get_session(session_id)

            if not batch_session:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail="Batch session not found",
                )

            if batch_session.user_id != current_user.id:
                raise HTTPException(
                    status_code=status.HTTP_403_FORBIDDEN,
                    detail="Access denied to this batch session",
                )

            return BatchStatusResponse(
                session_id=batch_session.id,
                operation_type=batch_session.operation_type.value,
                total_items=batch_session.total_items,
                processed_items=batch_session.processed_items,
                successful_items=batch_session.successful_items,
                failed_items=batch_session.failed_items,
                remaining_items=batch_session.remaining_items,
                progress_percentage=batch_session.progress_percentage,
                status=batch_session.status.value,
                is_complete=batch_session.is_complete,
                created_at=batch_session.created_at,
                updated_at=batch_session.updated_at,
                errors=batch_session.errors or [],
            )

        except HTTPException:
            raise
        except Exception as e:
            logger.error(
                f"Error getting batch status for session {session_id} "
                f"({config.source_name}): {str(e)}"
            )
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Failed to get batch status",
            )

    @router.delete(
        "/{session_id}",
        response_model=BatchCancelResponse,
        status_code=status.HTTP_200_OK,
    )
    async def cancel_batch_session(
        session_id: str,
        current_user: Annotated[Any, Depends(config.auth_dependency)],
    ) -> BatchCancelResponse:
        """Cancel a batch session."""
        try:
            logger.info(
                f"Cancelling batch session {session_id} for user "
                f"{current_user.id} ({config.source_name})"
            )

            session_manager = get_batch_session_manager()
            batch_session = session_manager.cancel_session(
                session_id, current_user.id
            )

            if not batch_session:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail="Batch session not found or access denied",
                )

            logger.info(
                f"Cancelled batch session {session_id}: "
                f"{batch_session.processed_items} processed, "
                f"{batch_session.successful_items} successful "
                f"({config.source_name})"
            )

            return BatchCancelResponse(
                session_id=batch_session.id,
                status=batch_session.status.value,
                processed_items=batch_session.processed_items,
                successful_items=batch_session.successful_items,
                failed_items=batch_session.failed_items,
                message=f"Batch session cancelled. Processed "
                f"{batch_session.processed_items} games with "
                f"{batch_session.successful_items} successful operations.",
            )

        except HTTPException:
            raise
        except Exception as e:
            logger.error(
                f"Error cancelling batch session {session_id} "
                f"({config.source_name}): {str(e)}"
            )
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Failed to cancel batch session",
            )

    return router


def _create_batch_response(
    _config: BatchSourceConfig,
    batch_session,
    current_batch_items: List,
    message: str,
) -> BatchNextResponse:
    """Helper function to create a consistent batch response."""
    return BatchNextResponse(
        session_id=batch_session.id,
        batch_processed=len(current_batch_items),
        batch_successful=0,
        batch_failed=0,
        batch_errors=[],
        current_batch_items=current_batch_items,
        total_items=batch_session.total_items,
        processed_items=batch_session.processed_items,
        successful_items=batch_session.successful_items,
        failed_items=batch_session.failed_items,
        remaining_items=batch_session.remaining_items,
        progress_percentage=batch_session.progress_percentage,
        status=batch_session.status.value,
        is_complete=batch_session.is_complete,
        message=message,
    )
