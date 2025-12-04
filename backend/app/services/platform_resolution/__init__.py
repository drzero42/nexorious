"""
Platform resolution service package.

This package provides platform and storefront resolution functionality
for CSV imports using fuzzy matching and explicit mappings.

Re-exports for backwards compatibility:
    - PlatformResolutionService: Main service class
    - create_platform_resolution_service: Factory function
    - EXPLICIT_PLATFORM_MAPPINGS: Platform name mappings
    - EXPLICIT_STOREFRONT_MAPPINGS: Storefront name mappings
"""

from .models import (
    EXPLICIT_PLATFORM_MAPPINGS,
    EXPLICIT_STOREFRONT_MAPPINGS,
    sanitize_platform_name,
)
from .service import PlatformResolutionService, create_platform_resolution_service

__all__ = [
    # Main service
    "PlatformResolutionService",
    # Factory function
    "create_platform_resolution_service",
    # Constants
    "EXPLICIT_PLATFORM_MAPPINGS",
    "EXPLICIT_STOREFRONT_MAPPINGS",
    # Utility functions
    "sanitize_platform_name",
]
