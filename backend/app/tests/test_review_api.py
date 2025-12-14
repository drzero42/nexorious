"""
Integration tests for Review queue API endpoints.

Tests the following endpoints:
- GET /api/review - List pending review items
- GET /api/review/summary - Get review item statistics
- GET /api/review/{item_id} - Get review item details with IGDB candidates
- POST /api/review/{item_id}/match - Match to IGDB ID
- POST /api/review/{item_id}/skip - Skip this game
- POST /api/review/{item_id}/keep - Keep game (for removals)
- POST /api/review/{item_id}/remove - Remove game (for removals)
- POST /api/jobs/{job_id}/confirm - Confirm import after review
"""

from sqlmodel import Session
from datetime import datetime, timezone

from ..models.user import User
from ..models.job import (
    Job,
    ReviewItem,
    BackgroundJobType,
    BackgroundJobSource,
    BackgroundJobStatus,
    ReviewItemStatus,
)


class TestListReviewItems:
    """Tests for GET /api/review endpoint."""

    def test_list_review_items_empty(self, client, auth_headers, test_user: User):
        """Test listing review items when user has none."""
        response = client.get("/api/review/", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 0
        assert data["items"] == []
        assert data["page"] == 1
        assert data["per_page"] == 20
        assert data["pages"] == 1

    def test_list_review_items_single_item(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test listing review items with a single item."""
        # Create job first
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.AWAITING_REVIEW,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create review item
        item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Half-Life 3",
            status=ReviewItemStatus.PENDING,
        )
        session.add(item)
        session.commit()

        response = client.get("/api/review/", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 1
        assert len(data["items"]) == 1
        assert data["items"][0]["source_title"] == "Half-Life 3"
        assert data["items"][0]["status"] == "pending"
        assert data["items"][0]["job_type"] == "import"
        assert data["items"][0]["job_source"] == "steam"

    def test_list_review_items_filter_by_status(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test filtering review items by status."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.AWAITING_REVIEW,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create items with different statuses
        for status in [
            ReviewItemStatus.PENDING,
            ReviewItemStatus.MATCHED,
            ReviewItemStatus.SKIPPED,
        ]:
            item = ReviewItem(
                job_id=job.id,
                user_id=test_user.id,
                source_title=f"Game {status.value}",
                status=status,
            )
            session.add(item)
        session.commit()

        response = client.get("/api/review/?status=pending", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 1
        assert data["items"][0]["status"] == "pending"

    def test_list_review_items_filter_by_job_id(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test filtering review items by job ID."""
        # Create two jobs
        job1 = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        job2 = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
        )
        session.add_all([job1, job2])
        session.commit()
        session.refresh(job1)
        session.refresh(job2)

        # Create items for each job
        item1 = ReviewItem(
            job_id=job1.id,
            user_id=test_user.id,
            source_title="Game 1",
        )
        item2 = ReviewItem(
            job_id=job2.id,
            user_id=test_user.id,
            source_title="Game 2",
        )
        session.add_all([item1, item2])
        session.commit()

        response = client.get(f"/api/review/?job_id={job1.id}", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 1
        assert data["items"][0]["source_title"] == "Game 1"

    def test_list_review_items_pagination(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test pagination of review items."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create 25 review items
        for i in range(25):
            item = ReviewItem(
                job_id=job.id,
                user_id=test_user.id,
                source_title=f"Game {i}",
            )
            session.add(item)
        session.commit()

        # First page
        response = client.get("/api/review/?page=1&per_page=10", headers=auth_headers)
        assert response.status_code == 200
        data = response.json()
        assert data["total"] == 25
        assert len(data["items"]) == 10
        assert data["pages"] == 3

        # Second page
        response = client.get("/api/review/?page=2&per_page=10", headers=auth_headers)
        assert response.status_code == 200
        data = response.json()
        assert len(data["items"]) == 10

    def test_list_review_items_only_own_items(
        self, client, auth_headers, test_user: User, admin_user: User, session: Session
    ):
        """Test that users only see their own review items."""
        # Create jobs for both users
        user_job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        admin_job = Job(
            user_id=admin_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add_all([user_job, admin_job])
        session.commit()
        session.refresh(user_job)
        session.refresh(admin_job)

        # Create items for each user
        user_item = ReviewItem(
            job_id=user_job.id,
            user_id=test_user.id,
            source_title="User Game",
        )
        admin_item = ReviewItem(
            job_id=admin_job.id,
            user_id=admin_user.id,
            source_title="Admin Game",
        )
        session.add_all([user_item, admin_item])
        session.commit()

        response = client.get("/api/review/", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total"] == 1
        assert data["items"][0]["source_title"] == "User Game"


class TestGetReviewSummary:
    """Tests for GET /api/review/summary endpoint."""

    def test_get_summary_empty(self, client, auth_headers, test_user: User):
        """Test getting summary with no review items."""
        response = client.get("/api/review/summary", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total_pending"] == 0
        assert data["total_matched"] == 0
        assert data["total_skipped"] == 0
        assert data["total_removal"] == 0
        assert data["jobs_with_pending"] == 0

    def test_get_summary_with_items(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test getting summary with various review items."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create items with different statuses
        statuses = [
            ReviewItemStatus.PENDING,
            ReviewItemStatus.PENDING,
            ReviewItemStatus.MATCHED,
            ReviewItemStatus.SKIPPED,
            ReviewItemStatus.REMOVAL,
        ]
        for i, status in enumerate(statuses):
            item = ReviewItem(
                job_id=job.id,
                user_id=test_user.id,
                source_title=f"Game {i}",
                status=status,
            )
            session.add(item)
        session.commit()

        response = client.get("/api/review/summary", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["total_pending"] == 2
        assert data["total_matched"] == 1
        assert data["total_skipped"] == 1
        assert data["total_removal"] == 1
        assert data["jobs_with_pending"] == 1


class TestGetReviewItem:
    """Tests for GET /api/review/{item_id} endpoint."""

    def test_get_review_item_success(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test getting a specific review item."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create item with IGDB candidates
        item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Portal",
        )
        item.set_igdb_candidates([
            {"igdb_id": 123, "name": "Portal", "first_release_date": 2007},
            {"igdb_id": 456, "name": "Portal 2", "first_release_date": 2011},
        ])
        item.set_source_metadata({"platform_id": "steam_400", "release_year": 2007})
        session.add(item)
        session.commit()
        session.refresh(item)

        response = client.get(f"/api/review/{item.id}", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["id"] == item.id
        assert data["source_title"] == "Portal"
        assert len(data["igdb_candidates"]) == 2
        assert data["igdb_candidates"][0]["igdb_id"] == 123
        assert data["source_metadata"]["platform_id"] == "steam_400"
        assert data["job_type"] == "import"
        assert data["job_source"] == "darkadia"

    def test_get_review_item_not_found(self, client, auth_headers):
        """Test getting a non-existent review item."""
        response = client.get("/api/review/nonexistent-id", headers=auth_headers)
        assert response.status_code == 404
        assert "not found" in response.json()["error"].lower()

    def test_get_review_item_other_user(
        self, client, auth_headers, test_user: User, admin_user: User, session: Session
    ):
        """Test that users cannot view other users' review items."""
        admin_job = Job(
            user_id=admin_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(admin_job)
        session.commit()
        session.refresh(admin_job)

        admin_item = ReviewItem(
            job_id=admin_job.id,
            user_id=admin_user.id,
            source_title="Admin Game",
        )
        session.add(admin_item)
        session.commit()
        session.refresh(admin_item)

        response = client.get(f"/api/review/{admin_item.id}", headers=auth_headers)
        assert response.status_code == 404


class TestMatchReviewItem:
    """Tests for POST /api/review/{item_id}/match endpoint."""

    def test_match_item_success(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test matching a review item to an IGDB ID."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Portal",
            status=ReviewItemStatus.PENDING,
        )
        session.add(item)
        session.commit()
        session.refresh(item)

        response = client.post(
            f"/api/review/{item.id}/match",
            headers=auth_headers,
            json={"igdb_id": 12345},
        )
        assert response.status_code == 200

        data = response.json()
        assert data["success"] is True
        assert "12345" in data["message"]
        assert data["item"]["status"] == "matched"
        assert data["item"]["resolved_igdb_id"] == 12345

        # Verify database state
        session.refresh(item)
        assert item.status == ReviewItemStatus.MATCHED
        assert item.resolved_igdb_id == 12345
        assert item.resolved_at is not None

    def test_match_item_already_resolved(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that already resolved items cannot be matched."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Portal",
            status=ReviewItemStatus.MATCHED,
            resolved_igdb_id=99999,
        )
        session.add(item)
        session.commit()
        session.refresh(item)

        response = client.post(
            f"/api/review/{item.id}/match",
            headers=auth_headers,
            json={"igdb_id": 12345},
        )
        assert response.status_code == 400
        assert "already resolved" in response.json()["error"].lower()

    def test_match_item_not_found(self, client, auth_headers):
        """Test matching a non-existent item."""
        response = client.post(
            "/api/review/nonexistent-id/match",
            headers=auth_headers,
            json={"igdb_id": 12345},
        )
        assert response.status_code == 404


class TestSkipReviewItem:
    """Tests for POST /api/review/{item_id}/skip endpoint."""

    def test_skip_item_success(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test skipping a review item."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Unknown Game",
            status=ReviewItemStatus.PENDING,
        )
        session.add(item)
        session.commit()
        session.refresh(item)

        response = client.post(f"/api/review/{item.id}/skip", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["success"] is True
        assert data["item"]["status"] == "skipped"

        # Verify database state
        session.refresh(item)
        assert item.status == ReviewItemStatus.SKIPPED
        assert item.resolved_at is not None

    def test_skip_item_already_resolved(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that already resolved items cannot be skipped."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Game",
            status=ReviewItemStatus.SKIPPED,
        )
        session.add(item)
        session.commit()
        session.refresh(item)

        response = client.post(f"/api/review/{item.id}/skip", headers=auth_headers)
        assert response.status_code == 400


class TestKeepReviewItem:
    """Tests for POST /api/review/{item_id}/keep endpoint."""

    def test_keep_item_success(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test keeping a game flagged for removal."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Removed Game",
            status=ReviewItemStatus.PENDING,
        )
        item.set_source_metadata({"removal_detected": True})
        session.add(item)
        session.commit()
        session.refresh(item)

        response = client.post(f"/api/review/{item.id}/keep", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["success"] is True
        assert "kept" in data["message"].lower()
        assert data["item"]["status"] == "matched"

        # Verify database state
        session.refresh(item)
        assert item.status == ReviewItemStatus.MATCHED
        metadata = item.get_source_metadata()
        assert metadata.get("kept_in_collection") is True


class TestRemoveReviewItem:
    """Tests for POST /api/review/{item_id}/remove endpoint."""

    def test_remove_item_success(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test removing a game flagged for removal."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Refunded Game",
            status=ReviewItemStatus.PENDING,
        )
        session.add(item)
        session.commit()
        session.refresh(item)

        response = client.post(f"/api/review/{item.id}/remove", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["success"] is True
        assert "removal" in data["message"].lower()
        assert data["item"]["status"] == "removal"

        # Verify database state
        session.refresh(item)
        assert item.status == ReviewItemStatus.REMOVAL


class TestConfirmImport:
    """Tests for POST /api/jobs/{job_id}/confirm endpoint."""

    def test_confirm_import_success(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test confirming an import after all items are reviewed."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.AWAITING_REVIEW,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create resolved review items
        items = [
            ReviewItem(
                job_id=job.id,
                user_id=test_user.id,
                source_title="Game 1",
                status=ReviewItemStatus.MATCHED,
                resolved_igdb_id=123,
            ),
            ReviewItem(
                job_id=job.id,
                user_id=test_user.id,
                source_title="Game 2",
                status=ReviewItemStatus.SKIPPED,
            ),
            ReviewItem(
                job_id=job.id,
                user_id=test_user.id,
                source_title="Game 3",
                status=ReviewItemStatus.MATCHED,
                resolved_igdb_id=456,
            ),
        ]
        session.add_all(items)
        session.commit()

        response = client.post(f"/api/jobs/{job.id}/confirm", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["success"] is True
        assert data["games_added"] == 2  # 2 matched
        assert data["games_skipped"] == 1
        assert data["games_removed"] == 0
        assert data["job"]["status"] == "completed"

        # Verify database state
        session.refresh(job)
        assert job.status == BackgroundJobStatus.COMPLETED
        assert job.completed_at is not None

    def test_confirm_import_with_pending_items(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that imports cannot be confirmed with pending items."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.AWAITING_REVIEW,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        # Create one pending item
        item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Pending Game",
            status=ReviewItemStatus.PENDING,
        )
        session.add(item)
        session.commit()

        response = client.post(f"/api/jobs/{job.id}/confirm", headers=auth_headers)
        assert response.status_code == 400
        assert "pending" in response.json()["error"].lower()

    def test_confirm_non_import_job(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that non-import jobs cannot be confirmed."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.SYNC,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.READY,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.post(f"/api/jobs/{job.id}/confirm", headers=auth_headers)
        assert response.status_code == 400
        assert "import" in response.json()["error"].lower()

    def test_confirm_wrong_status(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that jobs in wrong status cannot be confirmed."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.DARKADIA,
            status=BackgroundJobStatus.COMPLETED,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.post(f"/api/jobs/{job.id}/confirm", headers=auth_headers)
        assert response.status_code == 400

    def test_confirm_job_ready_status(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test confirming a job in 'ready' status."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.READY,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        response = client.post(f"/api/jobs/{job.id}/confirm", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()
        assert data["success"] is True

    def test_confirm_other_user_job(
        self, client, auth_headers, test_user: User, admin_user: User, session: Session
    ):
        """Test that users cannot confirm other users' jobs."""
        admin_job = Job(
            user_id=admin_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.STEAM,
            status=BackgroundJobStatus.READY,
        )
        session.add(admin_job)
        session.commit()
        session.refresh(admin_job)

        response = client.post(f"/api/jobs/{admin_job.id}/confirm", headers=auth_headers)
        assert response.status_code == 404


class TestReviewApiAuthentication:
    """Tests for authentication requirements on all review endpoints."""

    def test_list_review_items_no_auth(self, client):
        """Test that list review items requires authentication."""
        response = client.get("/api/review/")
        assert response.status_code == 403

    def test_get_review_item_no_auth(self, client):
        """Test that get review item requires authentication."""
        response = client.get("/api/review/some-id")
        assert response.status_code == 403

    def test_match_review_item_no_auth(self, client):
        """Test that match review item requires authentication."""
        response = client.post("/api/review/some-id/match", json={"igdb_id": 123})
        assert response.status_code == 403

    def test_skip_review_item_no_auth(self, client):
        """Test that skip review item requires authentication."""
        response = client.post("/api/review/some-id/skip")
        assert response.status_code == 403

    def test_keep_review_item_no_auth(self, client):
        """Test that keep review item requires authentication."""
        response = client.post("/api/review/some-id/keep")
        assert response.status_code == 403

    def test_remove_review_item_no_auth(self, client):
        """Test that remove review item requires authentication."""
        response = client.post("/api/review/some-id/remove")
        assert response.status_code == 403

    def test_confirm_job_no_auth(self, client):
        """Test that confirm job requires authentication."""
        response = client.post("/api/jobs/some-id/confirm")
        assert response.status_code == 403

    def test_review_summary_no_auth(self, client):
        """Test that review summary requires authentication."""
        response = client.get("/api/review/summary")
        assert response.status_code == 403


class TestReviewResponseFields:
    """Tests for review response field correctness."""

    def test_review_item_response_all_fields(
        self, client, auth_headers, test_user: User, session: Session
    ):
        """Test that all expected fields are in the response."""
        job = Job(
            user_id=test_user.id,
            job_type=BackgroundJobType.IMPORT,
            source=BackgroundJobSource.NEXORIOUS,
        )
        session.add(job)
        session.commit()
        session.refresh(job)

        now = datetime.now(timezone.utc)
        item = ReviewItem(
            job_id=job.id,
            user_id=test_user.id,
            source_title="Test Game",
            status=ReviewItemStatus.MATCHED,
            resolved_igdb_id=12345,
            resolved_at=now,
        )
        item.set_source_metadata({"platform_id": "steam_123"})
        item.set_igdb_candidates([{"igdb_id": 12345, "name": "Test Game"}])
        session.add(item)
        session.commit()
        session.refresh(item)

        response = client.get(f"/api/review/{item.id}", headers=auth_headers)
        assert response.status_code == 200

        data = response.json()

        # Verify all expected fields
        assert "id" in data
        assert "job_id" in data
        assert "user_id" in data
        assert "status" in data
        assert "source_title" in data
        assert "source_metadata" in data
        assert "igdb_candidates" in data
        assert "resolved_igdb_id" in data
        assert "created_at" in data
        assert "resolved_at" in data
        assert "job_type" in data
        assert "job_source" in data

        # Verify specific values
        assert data["source_title"] == "Test Game"
        assert data["resolved_igdb_id"] == 12345
        assert data["source_metadata"]["platform_id"] == "steam_123"
        assert data["job_type"] == "import"
        assert data["job_source"] == "nexorious"
