"""
Logo management service for platforms and storefronts.

Handles logo file upload, validation, storage, and cleanup operations.
"""

import shutil
import mimetypes
from pathlib import Path
from typing import Optional, List
from fastapi import HTTPException, UploadFile, status
import logging

logger = logging.getLogger(__name__)

class LogoService:
    """Service for managing platform and storefront logos."""
    
    def __init__(self, base_logo_dir: str = "static/logos"):
        """Initialize the logo service with base directory."""
        self.base_dir = Path(base_logo_dir)
        self.base_dir.mkdir(parents=True, exist_ok=True)
        
        # Supported file formats
        self.supported_formats = {
            'image/svg+xml': '.svg',
            'image/png': '.png',
            'image/jpeg': '.jpg',
            'image/webp': '.webp'
        }
        
        # Maximum file size (2MB)
        self.max_file_size = 2 * 1024 * 1024
    
    def _get_entity_dir(self, entity_type: str, entity_name: str) -> Path:
        """Get the directory path for a platform or storefront."""
        entity_dir = self.base_dir / entity_type / entity_name
        entity_dir.mkdir(parents=True, exist_ok=True)
        return entity_dir
    
    def _validate_file(self, file: UploadFile) -> str:
        """Validate uploaded file format and size."""
        # Check file size
        if file.size and file.size > self.max_file_size:
            raise HTTPException(
                status_code=status.HTTP_413_REQUEST_ENTITY_TOO_LARGE,
                detail=f"File size too large. Maximum size is {self.max_file_size / (1024*1024):.1f}MB"
            )
        
        # Read file content to check it's not empty
        file_content = file.file.read()
        file.file.seek(0)  # Reset file pointer
        
        if not file_content:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Empty file uploaded"
            )
        
        # Use filename and content-type to determine MIME type
        mime_type = file.content_type
        
        # Fallback to filename extension if content_type is not reliable
        if not mime_type or mime_type == 'application/octet-stream':
            if file.filename:
                guessed_type, _ = mimetypes.guess_type(file.filename)
                if guessed_type:
                    mime_type = guessed_type
        
        # Basic content validation for SVG files
        if mime_type == 'image/svg+xml' or (file.filename and file.filename.lower().endswith('.svg')):
            mime_type = 'image/svg+xml'
            # Check if file starts with SVG content
            if not file_content.startswith(b'<svg') and not file_content.startswith(b'<?xml'):
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Invalid SVG file format"
                )
        
        if mime_type is None or mime_type not in self.supported_formats:
            supported_list = list(self.supported_formats.keys())
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=f"Unsupported file format. Supported formats: {', '.join(supported_list)}"
            )

        return mime_type
    
    def _generate_filename(self, entity_name: str, theme: str, mime_type: str) -> str:
        """Generate filename for logo based on entity name and theme."""
        extension = self.supported_formats[mime_type]
        return f"{entity_name}-icon-{theme}{extension}"
    
    async def upload_logo(
        self, 
        entity_type: str,  # "platforms" or "storefronts"
        entity_name: str, 
        theme: str,  # "light" or "dark"
        file: UploadFile
    ) -> str:
        """
        Upload a logo file for a platform or storefront.
        
        Args:
            entity_type: "platforms" or "storefronts"
            entity_name: Name of the platform/storefront (used for directory)
            theme: "light" or "dark"
            file: Uploaded file
            
        Returns:
            Relative path to the uploaded file (for storing in database)
        """
        if entity_type not in ["platforms", "storefronts"]:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Invalid entity type. Must be 'platforms' or 'storefronts'"
            )
        
        if theme not in ["light", "dark"]:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Invalid theme. Must be 'light' or 'dark'"
            )
        
        # Validate file
        mime_type = self._validate_file(file)
        
        # Get entity directory
        entity_dir = self._get_entity_dir(entity_type, entity_name)
        
        # Generate filename
        filename = self._generate_filename(entity_name, theme, mime_type)
        file_path = entity_dir / filename
        
        try:
            # Save file
            with open(file_path, "wb") as buffer:
                shutil.copyfileobj(file.file, buffer)
            
            logger.info(f"Uploaded logo: {file_path}")
            
            # Return relative path for database storage
            return f"/static/logos/{entity_type}/{entity_name}/{filename}"
            
        except Exception as e:
            logger.error(f"Failed to save logo file {file_path}: {e}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail="Failed to save logo file"
            )
    
    def delete_logo(
        self,
        entity_type: str,  # "platforms" or "storefronts"
        entity_name: str,
        theme: Optional[str] = None  # If None, delete all themes
    ) -> List[str]:
        """
        Delete logo files for a platform or storefront.
        
        Args:
            entity_type: "platforms" or "storefronts"
            entity_name: Name of the platform/storefront
            theme: Specific theme to delete, or None to delete all
            
        Returns:
            List of deleted file paths
        """
        if entity_type not in ["platforms", "storefronts"]:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Invalid entity type. Must be 'platforms' or 'storefronts'"
            )
        
        entity_dir = self._get_entity_dir(entity_type, entity_name)
        deleted_files = []
        
        if theme:
            # Delete specific theme
            if theme not in ["light", "dark"]:
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Invalid theme. Must be 'light' or 'dark'"
                )
            
            # Find files matching the theme pattern
            pattern = f"{entity_name}-icon-{theme}.*"
            for file_path in entity_dir.glob(pattern):
                try:
                    file_path.unlink()
                    deleted_files.append(str(file_path))
                    logger.info(f"Deleted logo: {file_path}")
                except Exception as e:
                    logger.error(f"Failed to delete logo {file_path}: {e}")
        else:
            # Delete all theme files
            for theme_name in ["light", "dark"]:
                pattern = f"{entity_name}-icon-{theme_name}.*"
                for file_path in entity_dir.glob(pattern):
                    try:
                        file_path.unlink()
                        deleted_files.append(str(file_path))
                        logger.info(f"Deleted logo: {file_path}")
                    except Exception as e:
                        logger.error(f"Failed to delete logo {file_path}: {e}")
        
        return deleted_files
    
    def list_logos(self, entity_type: str, entity_name: str) -> List[dict]:
        """
        List available logo files for a platform or storefront.
        
        Args:
            entity_type: "platforms" or "storefronts"
            entity_name: Name of the platform/storefront
            
        Returns:
            List of logo file information
        """
        if entity_type not in ["platforms", "storefronts"]:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Invalid entity type. Must be 'platforms' or 'storefronts'"
            )
        
        entity_dir = self._get_entity_dir(entity_type, entity_name)
        logos = []
        
        for theme in ["light", "dark"]:
            pattern = f"{entity_name}-icon-{theme}.*"
            matching_files = list(entity_dir.glob(pattern))
            
            if matching_files:
                # Take the first matching file (there should only be one per theme)
                file_path = matching_files[0]
                relative_path = f"/static/logos/{entity_type}/{entity_name}/{file_path.name}"
                
                logos.append({
                    "theme": theme,
                    "filename": file_path.name,
                    "path": relative_path,
                    "size": file_path.stat().st_size,
                    "extension": file_path.suffix
                })
        
        return logos
    
    def cleanup_entity_logos(self, entity_type: str, entity_name: str) -> List[str]:
        """
        Clean up all logos for an entity when it's deleted.
        
        Args:
            entity_type: "platforms" or "storefronts"
            entity_name: Name of the platform/storefront
            
        Returns:
            List of deleted file paths
        """
        entity_dir = self._get_entity_dir(entity_type, entity_name)
        deleted_files = []
        
        if entity_dir.exists():
            try:
                # Remove all files in the directory
                for file_path in entity_dir.iterdir():
                    if file_path.is_file():
                        file_path.unlink()
                        deleted_files.append(str(file_path))
                
                # Remove the directory if it's empty
                if not any(entity_dir.iterdir()):
                    entity_dir.rmdir()
                    logger.info(f"Removed empty directory: {entity_dir}")
                
            except Exception as e:
                logger.error(f"Failed to cleanup logos for {entity_type}/{entity_name}: {e}")
        
        return deleted_files


# Global logo service instance
logo_service = LogoService()