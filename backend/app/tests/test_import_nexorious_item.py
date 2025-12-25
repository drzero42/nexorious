"""Tests for Nexorious item import task."""

import pytest
from unittest.mock import AsyncMock, patch
from sqlmodel import Session, select

from app.models.job import (
    Job,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    ImportJobSubtype,
)
from app.models.user import User


class TestImportNexoriousItem:
    """Tests for Nexorious item import task."""

    @pytest.mark.asyncio
    async def test_item_task_processes_game(self, session: Session, test_user: User):
        """Test that item task processes a game import."""
        # Create parent job
        parent = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(parent)
        session.commit()
        session.refresh(parent)

        # Create child job
        child = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            import_subtype=ImportJobSubtype.LIBRARY_IMPORT,
            status=BackgroundJobStatus.PENDING,
            parent_job_id=parent.id,
        )
        child.set_result_summary({
            "_item_data": {
                "title": "Test Game",
                "igdb_id": 12345,
                "play_status": "completed",
            },
            "title": "Test Game",
            "igdb_id": 12345,
        })
        session.add(child)
        session.commit()
        session.refresh(child)

        from app.worker.tasks.import_export.import_nexorious_item import import_nexorious_item

        # Mock the processing functions
        with patch(
            "app.worker.tasks.import_export.import_nexorious_item._process_nexorious_game"
        ) as mock_process:
            mock_process.return_value = "imported"

            # Mock get_session_context to return our test session
            with patch(
                "app.worker.tasks.import_export.import_nexorious_item.get_session_context"
            ) as mock_session:
                mock_session.return_value.__aenter__ = AsyncMock(return_value=session)
                mock_session.return_value.__aexit__ = AsyncMock()

                result = await import_nexorious_item(child.id)

        assert result["status"] == "success"
        assert result["result"] == "imported"

        # Verify child status updated
        session.refresh(child)
        assert child.status == BackgroundJobStatus.COMPLETED

    @pytest.mark.asyncio
    async def test_item_task_processes_wishlist(self, session: Session, test_user: User):
        """Test that item task processes a wishlist import."""
        # Create parent job
        parent = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(parent)
        session.commit()
        session.refresh(parent)

        # Create child job
        child = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            import_subtype=ImportJobSubtype.WISHLIST_IMPORT,
            status=BackgroundJobStatus.PENDING,
            parent_job_id=parent.id,
        )
        child.set_result_summary({
            "_item_data": {
                "title": "Wishlist Game",
                "igdb_id": 999,
            },
            "title": "Wishlist Game",
            "igdb_id": 999,
        })
        session.add(child)
        session.commit()
        session.refresh(child)

        from app.worker.tasks.import_export.import_nexorious_item import import_nexorious_item

        # Mock the processing functions
        with patch(
            "app.worker.tasks.import_export.import_nexorious_item._process_wishlist_item"
        ) as mock_process:
            mock_process.return_value = "imported"

            with patch(
                "app.worker.tasks.import_export.import_nexorious_item.get_session_context"
            ) as mock_session:
                mock_session.return_value.__aenter__ = AsyncMock(return_value=session)
                mock_session.return_value.__aexit__ = AsyncMock()

                result = await import_nexorious_item(child.id)

        assert result["status"] == "success"
        assert result["result"] == "imported"

        # Verify child status updated
        session.refresh(child)
        assert child.status == BackgroundJobStatus.COMPLETED

    @pytest.mark.asyncio
    async def test_item_task_finalizes_parent_when_all_complete(self, session: Session, test_user: User):
        """Test that last child finalizes parent job."""
        # Create parent job
        parent = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(parent)
        session.commit()
        session.refresh(parent)

        # Create one already-completed child
        completed_child = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            import_subtype=ImportJobSubtype.LIBRARY_IMPORT,
            status=BackgroundJobStatus.COMPLETED,
            parent_job_id=parent.id,
        )
        session.add(completed_child)

        # Create the child we're about to process (the last one)
        last_child = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            import_subtype=ImportJobSubtype.LIBRARY_IMPORT,
            status=BackgroundJobStatus.PENDING,
            parent_job_id=parent.id,
        )
        last_child.set_result_summary({
            "_item_data": {"title": "Last Game", "igdb_id": 999},
            "title": "Last Game",
            "igdb_id": 999,
        })
        session.add(last_child)
        session.commit()
        session.refresh(last_child)

        from app.worker.tasks.import_export.import_nexorious_item import import_nexorious_item

        with patch(
            "app.worker.tasks.import_export.import_nexorious_item._process_nexorious_game"
        ) as mock_process:
            mock_process.return_value = "imported"

            with patch(
                "app.worker.tasks.import_export.import_nexorious_item.get_session_context"
            ) as mock_session:
                mock_session.return_value.__aenter__ = AsyncMock(return_value=session)
                mock_session.return_value.__aexit__ = AsyncMock()

                await import_nexorious_item(last_child.id)

        # Parent should be finalized
        session.refresh(parent)
        assert parent.status == BackgroundJobStatus.COMPLETED

    @pytest.mark.asyncio
    async def test_item_task_does_not_finalize_parent_when_siblings_pending(
        self, session: Session, test_user: User
    ):
        """Test that parent is not finalized when siblings are still pending."""
        # Create parent job
        parent = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(parent)
        session.commit()
        session.refresh(parent)

        # Create one pending sibling
        pending_sibling = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            import_subtype=ImportJobSubtype.LIBRARY_IMPORT,
            status=BackgroundJobStatus.PENDING,
            parent_job_id=parent.id,
        )
        session.add(pending_sibling)

        # Create the child we're about to process
        child = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            import_subtype=ImportJobSubtype.LIBRARY_IMPORT,
            status=BackgroundJobStatus.PENDING,
            parent_job_id=parent.id,
        )
        child.set_result_summary({
            "_item_data": {"title": "Game", "igdb_id": 123},
            "title": "Game",
            "igdb_id": 123,
        })
        session.add(child)
        session.commit()
        session.refresh(child)

        from app.worker.tasks.import_export.import_nexorious_item import import_nexorious_item

        with patch(
            "app.worker.tasks.import_export.import_nexorious_item._process_nexorious_game"
        ) as mock_process:
            mock_process.return_value = "imported"

            with patch(
                "app.worker.tasks.import_export.import_nexorious_item.get_session_context"
            ) as mock_session:
                mock_session.return_value.__aenter__ = AsyncMock(return_value=session)
                mock_session.return_value.__aexit__ = AsyncMock()

                await import_nexorious_item(child.id)

        # Parent should NOT be finalized (sibling still pending)
        session.refresh(parent)
        assert parent.status == BackgroundJobStatus.PROCESSING

    @pytest.mark.asyncio
    async def test_item_task_handles_processing_error(self, session: Session, test_user: User):
        """Test that item task handles processing errors gracefully."""
        # Create parent job
        parent = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(parent)
        session.commit()
        session.refresh(parent)

        # Create child job
        child = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            import_subtype=ImportJobSubtype.LIBRARY_IMPORT,
            status=BackgroundJobStatus.PENDING,
            parent_job_id=parent.id,
        )
        child.set_result_summary({
            "_item_data": {"title": "Failing Game", "igdb_id": 456},
            "title": "Failing Game",
            "igdb_id": 456,
        })
        session.add(child)
        session.commit()
        session.refresh(child)

        from app.worker.tasks.import_export.import_nexorious_item import import_nexorious_item

        # Mock the processing function to raise an error
        with patch(
            "app.worker.tasks.import_export.import_nexorious_item._process_nexorious_game"
        ) as mock_process:
            mock_process.side_effect = Exception("Processing failed")

            with patch(
                "app.worker.tasks.import_export.import_nexorious_item.get_session_context"
            ) as mock_session:
                mock_session.return_value.__aenter__ = AsyncMock(return_value=session)
                mock_session.return_value.__aexit__ = AsyncMock()

                result = await import_nexorious_item(child.id)

        assert result["status"] == "error"
        assert "Processing failed" in result["error"]

        # Verify child status is FAILED
        session.refresh(child)
        assert child.status == BackgroundJobStatus.FAILED
        assert child.error_message is not None

    @pytest.mark.asyncio
    async def test_item_task_handles_missing_item_data(self, session: Session, test_user: User):
        """Test that item task handles missing item data."""
        # Create parent job
        parent = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PROCESSING,
        )
        session.add(parent)
        session.commit()
        session.refresh(parent)

        # Create child job without _item_data
        child = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            import_subtype=ImportJobSubtype.LIBRARY_IMPORT,
            status=BackgroundJobStatus.PENDING,
            parent_job_id=parent.id,
        )
        child.set_result_summary({})
        session.add(child)
        session.commit()
        session.refresh(child)

        from app.worker.tasks.import_export.import_nexorious_item import import_nexorious_item

        with patch(
            "app.worker.tasks.import_export.import_nexorious_item.get_session_context"
        ) as mock_session:
            mock_session.return_value.__aenter__ = AsyncMock(return_value=session)
            mock_session.return_value.__aexit__ = AsyncMock()

            result = await import_nexorious_item(child.id)

        assert result["status"] == "error"
        assert "No item data found" in result["error"]

        # Verify child status is FAILED
        session.refresh(child)
        assert child.status == BackgroundJobStatus.FAILED
