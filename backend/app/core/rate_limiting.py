"""
Rate limiting utilities for API endpoints.

Implements rate limiting for platform resolution endpoints to prevent abuse
and resource exhaustion attacks.
"""

import time
import logging
from typing import Dict, Optional
from functools import wraps
from fastapi import HTTPException, status, Request
from datetime import datetime, timedelta

logger = logging.getLogger(__name__)


class RateLimiter:
    """Simple in-memory rate limiter using sliding window."""
    
    def __init__(self, max_requests: int, window_seconds: int):
        self.max_requests = max_requests
        self.window_seconds = window_seconds
        self.requests: Dict[str, list] = {}  # user_id -> [timestamps]
    
    def is_allowed(self, user_id: str) -> tuple[bool, Optional[int]]:
        """
        Check if request is allowed for user.
        
        Returns:
            Tuple of (is_allowed, seconds_until_reset)
        """
        now = time.time()
        window_start = now - self.window_seconds
        
        # Get user's request history
        if user_id not in self.requests:
            self.requests[user_id] = []
        
        user_requests = self.requests[user_id]
        
        # Remove requests outside the window
        user_requests[:] = [req_time for req_time in user_requests if req_time > window_start]
        
        # Check if under limit
        if len(user_requests) < self.max_requests:
            # Add current request and allow
            user_requests.append(now)
            return True, None
        else:
            # Calculate seconds until oldest request expires
            oldest_request = min(user_requests)
            seconds_until_reset = int(oldest_request + self.window_seconds - now)
            return False, max(1, seconds_until_reset)
    
    def cleanup_old_entries(self):
        """Cleanup old entries to prevent memory leak."""
        now = time.time()
        window_start = now - self.window_seconds
        
        # Remove users with no recent requests
        users_to_remove = []
        for user_id, user_requests in self.requests.items():
            user_requests[:] = [req_time for req_time in user_requests if req_time > window_start]
            if not user_requests:
                users_to_remove.append(user_id)
        
        for user_id in users_to_remove:
            del self.requests[user_id]


# Rate limiters for different endpoint types
platform_suggestions_limiter = RateLimiter(max_requests=30, window_seconds=60)  # 30 requests per minute
platform_resolution_limiter = RateLimiter(max_requests=20, window_seconds=60)   # 20 resolutions per minute
bulk_resolution_limiter = RateLimiter(max_requests=5, window_seconds=60)        # 5 bulk operations per minute


def rate_limit(limiter: RateLimiter, operation_name: str):
    """
    Decorator to apply rate limiting to endpoints.
    
    Args:
        limiter: RateLimiter instance to use
        operation_name: Name of operation for logging
    """
    def decorator(func):
        @wraps(func)
        async def wrapper(*args, **kwargs):
            # Find current_user in the function arguments
            current_user = None
            for arg in args:
                if hasattr(arg, 'id') and hasattr(arg, 'username'):  # User object
                    current_user = arg
                    break
            
            # Also check kwargs
            if not current_user and 'current_user' in kwargs:
                current_user = kwargs['current_user']
            
            if not current_user:
                # If no user found, skip rate limiting (should not happen with proper auth)
                logger.warning(f"Rate limiting skipped for {operation_name} - no user found")
                return await func(*args, **kwargs)
            
            user_id = current_user.id
            is_allowed, seconds_until_reset = limiter.is_allowed(user_id)
            
            if not is_allowed:
                logger.warning(f"Rate limit exceeded for user {user_id} on {operation_name}")
                
                # Try to get audit logger and log the event
                try:
                    from ..core.audit_logging import audit
                    audit.log_rate_limit_exceeded(user_id, operation_name)
                except ImportError:
                    # If audit logging is not available, just log normally
                    pass
                
                raise HTTPException(
                    status_code=status.HTTP_429_TOO_MANY_REQUESTS,
                    detail=f"Rate limit exceeded for {operation_name}. Try again in {seconds_until_reset} seconds.",
                    headers={"Retry-After": str(seconds_until_reset)}
                )
            
            # Periodic cleanup to prevent memory leak
            import random
            if random.random() < 0.01:  # 1% chance
                limiter.cleanup_old_entries()
            
            return await func(*args, **kwargs)
        return wrapper
    return decorator


# Convenience decorators for different operations
def rate_limit_suggestions(func):
    """Rate limit platform suggestion requests."""
    return rate_limit(platform_suggestions_limiter, "platform suggestions")(func)


def rate_limit_resolution(func):
    """Rate limit platform resolution requests."""
    return rate_limit(platform_resolution_limiter, "platform resolution")(func)


def rate_limit_bulk_resolution(func):
    """Rate limit bulk platform resolution requests."""
    return rate_limit(bulk_resolution_limiter, "bulk platform resolution")(func)