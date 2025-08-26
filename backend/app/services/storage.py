"""
File storage service for cover art and other assets.
"""

import os
import logging
from pathlib import Path
from typing import Optional
import httpx
import tempfile
from PIL import Image

from app.core.config import settings

logger = logging.getLogger(__name__)


class StorageService:
    """Service for managing file storage operations."""
    
    def __init__(self):
        self.storage_path = Path(settings.storage_path or "storage")
        self.cover_art_path = self.storage_path / "cover_art"
        self.ensure_directories()
        
    def ensure_directories(self):
        """Ensure storage directories exist."""
        self.storage_path.mkdir(parents=True, exist_ok=True)
        self.cover_art_path.mkdir(parents=True, exist_ok=True)
        
    def get_cover_art_filename(self, igdb_id: str, url: str) -> str:
        """Generate filename for cover art based on IGDB ID and URL."""
        # Extract file extension from URL
        url_path = url.split('/')[-1]
        if '.' in url_path:
            ext = url_path.split('.')[-1]
        else:
            ext = 'jpg'  # Default extension
        
        # Create filename using IGDB ID
        return f"{igdb_id}.{ext}"
        
    def get_cover_art_path(self, igdb_id: str, url: str) -> Path:
        """Get full path for cover art file."""
        filename = self.get_cover_art_filename(igdb_id, url)
        return self.cover_art_path / filename
        
    def get_cover_art_url(self, igdb_id: str, url: str) -> str:
        """Get local URL for cover art file."""
        filename = self.get_cover_art_filename(igdb_id, url)
        return f"/static/cover_art/{filename}"
        
    async def download_and_store_cover_art(self, igdb_id: str, cover_url: str, max_retries: int = 3) -> Optional[str]:
        """Download cover art and store locally with validation and retry logic. Returns local URL on success."""
        if not cover_url or not igdb_id:
            logger.warning(f"Invalid parameters: igdb_id={igdb_id}, cover_url={cover_url}")
            return None
            
        # Validate URL format
        if not self._is_valid_url(cover_url):
            logger.error(f"Invalid cover art URL format: {cover_url}")
            return None
            
        # Check if file already exists and is valid
        file_path = self.get_cover_art_path(igdb_id, cover_url)
        if file_path.exists():
            if self._validate_image_file(file_path):
                logger.info(f"Valid cover art already exists for IGDB ID {igdb_id}")
                return self.get_cover_art_url(igdb_id, cover_url)
            else:
                logger.warning(f"Existing cover art is invalid, re-downloading for IGDB ID {igdb_id}")
                # Remove invalid file
                try:
                    file_path.unlink()
                except Exception as e:
                    logger.error(f"Failed to remove invalid file: {e}")
        
        # Download with retry logic
        for attempt in range(max_retries):
            try:
                return await self._download_with_validation(igdb_id, cover_url, file_path)
            except httpx.HTTPError as e:
                logger.warning(f"HTTP error on attempt {attempt + 1}/{max_retries} for {cover_url}: {e}")
                if attempt == max_retries - 1:
                    logger.error(f"Failed to download cover art after {max_retries} attempts: {e}")
                    return None
            except Exception as e:
                logger.error(f"Unexpected error downloading cover art for IGDB ID {igdb_id}: {e}")
                return None
                
        return None
    
    def _is_valid_url(self, url: str) -> bool:
        """Validate URL format."""
        try:
            return url.startswith(('http://', 'https://')) and len(url) > 10
        except Exception:
            return False
    
    def _validate_image_file(self, file_path: Path) -> bool:
        """Validate that file is a valid image."""
        try:
            if not file_path.exists() or file_path.stat().st_size == 0:
                return False
            
            # Try to open with PIL to validate it's a valid image
            with Image.open(file_path) as img:
                img.verify()
                return True
        except Exception:
            return False
    
    async def _download_with_validation(self, igdb_id: str, cover_url: str, file_path: Path) -> Optional[str]:
        """Download image with validation and atomic write."""
        # Create temporary file for atomic write
        temp_file = None
        try:
            # Download the image with timeout
            async with httpx.AsyncClient(timeout=30.0) as client:
                response = await client.get(cover_url)
                response.raise_for_status()
                
                # Validate content type
                content_type = response.headers.get('content-type', '')
                if not content_type.startswith('image/'):
                    logger.error(f"Invalid content type for cover art: {content_type}")
                    return None
                
                # Validate content length
                content_length = len(response.content)
                if content_length == 0:
                    logger.error("Empty response content")
                    return None
                
                if content_length > 10 * 1024 * 1024:  # 10MB limit
                    logger.error(f"Cover art too large: {content_length} bytes")
                    return None
                
                # Create temporary file in same directory for atomic move
                temp_file = tempfile.NamedTemporaryFile(
                    dir=file_path.parent,
                    delete=False,
                    suffix='.tmp'
                )
                
                # Write content to temporary file
                temp_file.write(response.content)
                temp_file.close()
                
                # Validate the downloaded image
                if not self._validate_image_file(Path(temp_file.name)):
                    logger.error(f"Downloaded file is not a valid image: {cover_url}")
                    os.unlink(temp_file.name)
                    return None
                
                # Atomic move to final location
                os.rename(temp_file.name, file_path)
                temp_file = None  # Prevent cleanup
                
                logger.info(f"Successfully downloaded and validated cover art for IGDB ID {igdb_id}")
                return self.get_cover_art_url(igdb_id, cover_url)
                
        except httpx.TimeoutException:
            logger.error(f"Timeout downloading cover art from {cover_url}")
            raise
        except httpx.HTTPError as e:
            logger.error(f"HTTP error downloading cover art from {cover_url}: {e}")
            raise
        except Exception as e:
            logger.error(f"Unexpected error downloading cover art: {e}")
            raise
        finally:
            # Cleanup temporary file if it exists
            if temp_file and os.path.exists(temp_file.name):
                try:
                    os.unlink(temp_file.name)
                except Exception as e:
                    logger.error(f"Failed to cleanup temporary file: {e}")
            
    def delete_cover_art(self, igdb_id: str, url: str) -> bool:
        """Delete cover art file."""
        try:
            file_path = self.get_cover_art_path(igdb_id, url)
            if file_path.exists():
                file_path.unlink()
                logger.info(f"Deleted cover art for IGDB ID {igdb_id}")
                return True
            return False
        except Exception as e:
            logger.error(f"Error deleting cover art for IGDB ID {igdb_id}: {e}")
            return False
            
    def cover_art_exists(self, igdb_id: str, url: str) -> bool:
        """Check if cover art file exists locally."""
        file_path = self.get_cover_art_path(igdb_id, url)
        return file_path.exists()
        
    def get_storage_stats(self) -> dict:
        """Get storage statistics."""
        try:
            cover_art_files = list(self.cover_art_path.glob("*"))
            total_size = sum(f.stat().st_size for f in cover_art_files if f.is_file())
            
            return {
                "cover_art_files": len(cover_art_files),
                "total_size_bytes": total_size,
                "total_size_mb": round(total_size / (1024 * 1024), 2)
            }
        except Exception as e:
            logger.error(f"Error getting storage stats: {e}")
            return {"error": str(e)}
            
    def cleanup_orphaned_files(self, valid_igdb_ids: list) -> dict:
        """Remove cover art files that don't correspond to any game."""
        try:
            removed_files = []
            total_size_removed = 0
            
            for file_path in self.cover_art_path.glob("*"):
                if file_path.is_file():
                    # Extract IGDB ID from filename
                    igdb_id = file_path.stem
                    if igdb_id not in valid_igdb_ids:
                        file_size = file_path.stat().st_size
                        file_path.unlink()
                        removed_files.append(file_path.name)
                        total_size_removed += file_size
                        
            return {
                "removed_files": len(removed_files),
                "total_size_removed_bytes": total_size_removed,
                "total_size_removed_mb": round(total_size_removed / (1024 * 1024), 2),
                "files": removed_files
            }
        except Exception as e:
            logger.error(f"Error during cleanup: {e}")
            return {"error": str(e)}


# Global instance
storage_service = StorageService()