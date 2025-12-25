"""Tests for Nexorious import coordinator task."""

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


class TestImportNexoriousCoordinator:
    """Tests for Nexorious import coordinator task."""

    @pytest.mark.asyncio
    async def test_coordinator_creates_child_jobs(self, session: Session, test_user: User):
        """Test that coordinator creates child jobs for each game and wishlist item."""
        # Create parent job with import data
        parent = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PENDING,
        )
        parent.set_result_summary({
            "_import_data": {
                "export_version": "1.2",
                "games": [
                    {"title": "Game 1", "igdb_id": 123},
                    {"title": "Game 2", "igdb_id": 456},
                ],
                "wishlist": [
                    {"title": "Wishlist Game", "igdb_id": 789},
                ],
            }
        })
        session.add(parent)
        session.commit()
        session.refresh(parent)

        # Import the coordinator
        from app.worker.tasks.import_export.import_nexorious_coordinator import (
            import_nexorious_coordinator,
        )

        # Mock get_session_context to use the test session
        with patch(
            "app.worker.tasks.import_export.import_nexorious_coordinator.get_session_context"
        ) as mock_context, patch(
            "app.worker.tasks.import_export.import_nexorious_item.import_nexorious_item"
        ) as mock_item_task:
            mock_context.return_value.__aenter__ = AsyncMock(return_value=session)
            mock_context.return_value.__aexit__ = AsyncMock()
            mock_item_task.kiq = AsyncMock()

            # Run coordinator
            result = await import_nexorious_coordinator(parent.id)

        assert result["status"] == "success"
        assert result["children_created"] == 3

        # Verify children were created
        session.expire_all()
        children = session.exec(
            select(Job).where(Job.parent_job_id == parent.id)
        ).all()

        assert len(children) == 3

        # Check game children
        game_children = [c for c in children if c.import_subtype == ImportJobSubtype.LIBRARY_IMPORT]
        assert len(game_children) == 2

        # Check wishlist children
        wishlist_children = [c for c in children if c.import_subtype == ImportJobSubtype.WISHLIST_IMPORT]
        assert len(wishlist_children) == 1

        # Verify _import_data was cleared from parent
        session.refresh(parent)
        result_summary = parent.get_result_summary()
        assert "_import_data" not in result_summary

    @pytest.mark.asyncio
    async def test_coordinator_with_no_import_data(self, session: Session, test_user: User):
        """Test coordinator handles missing import data."""
        # Create parent job without import data
        parent = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PENDING,
        )
        parent.set_result_summary({})
        session.add(parent)
        session.commit()
        session.refresh(parent)

        from app.worker.tasks.import_export.import_nexorious_coordinator import (
            import_nexorious_coordinator,
        )

        # Mock get_session_context to use the test session
        with patch(
            "app.worker.tasks.import_export.import_nexorious_coordinator.get_session_context"
        ) as mock_context:
            mock_context.return_value.__aenter__ = AsyncMock(return_value=session)
            mock_context.return_value.__aexit__ = AsyncMock()

            # Run coordinator
            result = await import_nexorious_coordinator(parent.id)

        assert result["status"] == "error"
        assert "No import data found" in result["error"]

        # Verify parent job failed
        session.refresh(parent)
        assert parent.status == BackgroundJobStatus.FAILED

    @pytest.mark.asyncio
    async def test_coordinator_with_empty_data(self, session: Session, test_user: User):
        """Test coordinator handles empty games and wishlist."""
        # Create parent job with empty import data
        parent = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PENDING,
        )
        parent.set_result_summary({
            "_import_data": {
                "export_version": "1.2",
                "games": [],
                "wishlist": [],
            }
        })
        session.add(parent)
        session.commit()
        session.refresh(parent)

        from app.worker.tasks.import_export.import_nexorious_coordinator import (
            import_nexorious_coordinator,
        )

        # Mock get_session_context to use the test session
        with patch(
            "app.worker.tasks.import_export.import_nexorious_coordinator.get_session_context"
        ) as mock_context, patch(
            "app.worker.tasks.import_export.import_nexorious_item.import_nexorious_item"
        ) as mock_item_task:
            mock_context.return_value.__aenter__ = AsyncMock(return_value=session)
            mock_context.return_value.__aexit__ = AsyncMock()
            mock_item_task.kiq = AsyncMock()

            # Run coordinator
            result = await import_nexorious_coordinator(parent.id)

        assert result["status"] == "success"
        assert result["children_created"] == 0
        assert result["games_count"] == 0
        assert result["wishlist_count"] == 0

        # Verify no children were created
        session.expire_all()
        children = session.exec(
            select(Job).where(Job.parent_job_id == parent.id)
        ).all()

        assert len(children) == 0

    @pytest.mark.asyncio
    async def test_coordinator_stores_item_data_in_children(self, session: Session, test_user: User):
        """Test that item data is stored in child job result_summary."""
        # Create parent job with import data
        parent = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PENDING,
        )
        parent.set_result_summary({
            "_import_data": {
                "export_version": "1.2",
                "games": [
                    {
                        "title": "Test Game",
                        "igdb_id": 999,
                        "play_status": "completed",
                        "personal_rating": 8.5,
                    },
                ],
                "wishlist": [],
            }
        })
        session.add(parent)
        session.commit()
        session.refresh(parent)

        from app.worker.tasks.import_export.import_nexorious_coordinator import (
            import_nexorious_coordinator,
        )

        # Mock get_session_context to use the test session
        with patch(
            "app.worker.tasks.import_export.import_nexorious_coordinator.get_session_context"
        ) as mock_context, patch(
            "app.worker.tasks.import_export.import_nexorious_item.import_nexorious_item"
        ) as mock_item_task:
            mock_context.return_value.__aenter__ = AsyncMock(return_value=session)
            mock_context.return_value.__aexit__ = AsyncMock()
            mock_item_task.kiq = AsyncMock()

            # Run coordinator
            await import_nexorious_coordinator(parent.id)

        # Verify child has the item data
        session.expire_all()
        children = session.exec(
            select(Job).where(Job.parent_job_id == parent.id)
        ).all()

        assert len(children) == 1
        child = children[0]

        result_summary = child.get_result_summary()
        assert "_item_data" in result_summary
        assert result_summary["_item_data"]["title"] == "Test Game"
        assert result_summary["_item_data"]["igdb_id"] == 999
        assert result_summary["_item_data"]["play_status"] == "completed"
        assert result_summary["title"] == "Test Game"
        assert result_summary["igdb_id"] == 999

    @pytest.mark.asyncio
    async def test_coordinator_enqueues_child_tasks(self, session: Session, test_user: User):
        """Test that coordinator enqueues child tasks."""
        # Create parent job with import data
        parent = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
            status=BackgroundJobStatus.PENDING,
        )
        parent.set_result_summary({
            "_import_data": {
                "export_version": "1.2",
                "games": [
                    {"title": "Game 1", "igdb_id": 123},
                ],
                "wishlist": [
                    {"title": "Wishlist Game", "igdb_id": 789},
                ],
            }
        })
        session.add(parent)
        session.commit()
        session.refresh(parent)

        from app.worker.tasks.import_export.import_nexorious_coordinator import (
            import_nexorious_coordinator,
        )

        # Mock get_session_context to use the test session
        with patch(
            "app.worker.tasks.import_export.import_nexorious_coordinator.get_session_context"
        ) as mock_context, patch(
            "app.worker.tasks.import_export.import_nexorious_item.import_nexorious_item"
        ) as mock_item_task:
            mock_context.return_value.__aenter__ = AsyncMock(return_value=session)
            mock_context.return_value.__aexit__ = AsyncMock()
            mock_item_task.kiq = AsyncMock()

            # Run coordinator
            await import_nexorious_coordinator(parent.id)

            # Verify child tasks were enqueued (once for each game + wishlist)
            assert mock_item_task.kiq.call_count == 2
