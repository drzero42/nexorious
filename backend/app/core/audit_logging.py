"""
Audit logging for security-sensitive operations.

Provides structured logging for platform resolution operations
for security monitoring and compliance.
"""

import logging
from typing import Optional, Dict, Any
from datetime import datetime, timezone
from enum import Enum

# Create a dedicated logger for audit events
audit_logger = logging.getLogger("nexorious.audit")


class AuditEventType(Enum):
    """Types of audit events."""
    PLATFORM_SUGGESTION_REQUEST = "platform_suggestion_request"
    PLATFORM_RESOLUTION = "platform_resolution"
    BULK_PLATFORM_RESOLUTION = "bulk_platform_resolution"
    PLATFORM_RESOLUTION_FAILURE = "platform_resolution_failure"
    RATE_LIMIT_EXCEEDED = "rate_limit_exceeded"
    INVALID_INPUT = "invalid_input"
    UNAUTHORIZED_ACCESS = "unauthorized_access"


class AuditLogger:
    """Centralized audit logging for platform resolution operations."""
    
    def __init__(self):
        self.logger = audit_logger
    
    def log_event(
        self,
        event_type: AuditEventType,
        user_id: str,
        description: str,
        details: Optional[Dict[str, Any]] = None,
        request_ip: Optional[str] = None,
        user_agent: Optional[str] = None,
        success: bool = True
    ):
        """
        Log an audit event.
        
        Args:
            event_type: Type of event being logged
            user_id: ID of the user performing the action
            description: Human-readable description of the event
            details: Additional structured data about the event
            request_ip: IP address of the request (if available)
            user_agent: User agent string (if available)
            success: Whether the operation was successful
        """
        audit_data = {
            "event_type": event_type.value,
            "user_id": user_id,
            "description": description,
            "success": success,
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "details": details or {},
        }
        
        if request_ip:
            audit_data["request_ip"] = request_ip
        
        if user_agent:
            audit_data["user_agent"] = user_agent
        
        # Log at different levels based on success and event type
        if not success or event_type in [
            AuditEventType.RATE_LIMIT_EXCEEDED,
            AuditEventType.UNAUTHORIZED_ACCESS,
            AuditEventType.INVALID_INPUT
        ]:
            self.logger.warning(f"AUDIT: {description}", extra=audit_data)
        else:
            self.logger.info(f"AUDIT: {description}", extra=audit_data)
    
    def log_platform_suggestion(
        self,
        user_id: str,
        unknown_platform_name: str,
        suggestions_count: int,
        request_ip: Optional[str] = None,
        min_confidence: float = 0.6
    ):
        """Log platform suggestion request."""
        self.log_event(
            event_type=AuditEventType.PLATFORM_SUGGESTION_REQUEST,
            user_id=user_id,
            description=f"User requested platform suggestions for '{unknown_platform_name}'",
            details={
                "unknown_platform_name": unknown_platform_name,
                "suggestions_count": suggestions_count,
                "min_confidence": min_confidence
            },
            request_ip=request_ip
        )
    
    def log_platform_resolution(
        self,
        user_id: str,
        import_id: str,
        original_platform_name: str,
        resolved_platform: Optional[str],
        resolved_storefront: Optional[str],
        success: bool,
        error_message: Optional[str] = None,
        request_ip: Optional[str] = None
    ):
        """Log platform resolution attempt."""
        description = f"User resolved platform '{original_platform_name}'"
        if not success:
            description = f"Failed to resolve platform '{original_platform_name}'"

        details = {
            "import_id": import_id,
            "original_platform_name": original_platform_name,
            "resolved_platform": resolved_platform,
            "resolved_storefront": resolved_storefront
        }
        
        if error_message:
            details["error_message"] = error_message
        
        self.log_event(
            event_type=AuditEventType.PLATFORM_RESOLUTION if success else AuditEventType.PLATFORM_RESOLUTION_FAILURE,
            user_id=user_id,
            description=description,
            details=details,
            request_ip=request_ip,
            success=success
        )
    
    def log_bulk_resolution(
        self,
        user_id: str,
        total_processed: int,
        successful_count: int,
        failed_count: int,
        request_ip: Optional[str] = None
    ):
        """Log bulk platform resolution operation."""
        self.log_event(
            event_type=AuditEventType.BULK_PLATFORM_RESOLUTION,
            user_id=user_id,
            description=f"User performed bulk resolution of {total_processed} platforms",
            details={
                "total_processed": total_processed,
                "successful_count": successful_count,
                "failed_count": failed_count
            },
            request_ip=request_ip,
            success=failed_count == 0
        )
    
    def log_rate_limit_exceeded(
        self,
        user_id: str,
        operation_name: str,
        request_ip: Optional[str] = None,
        user_agent: Optional[str] = None
    ):
        """Log rate limit exceeded event."""
        self.log_event(
            event_type=AuditEventType.RATE_LIMIT_EXCEEDED,
            user_id=user_id,
            description=f"Rate limit exceeded for {operation_name}",
            details={"operation_name": operation_name},
            request_ip=request_ip,
            user_agent=user_agent,
            success=False
        )
    
    def log_invalid_input(
        self,
        user_id: str,
        operation_name: str,
        invalid_input: str,
        request_ip: Optional[str] = None
    ):
        """Log invalid input attempt."""
        self.log_event(
            event_type=AuditEventType.INVALID_INPUT,
            user_id=user_id,
            description=f"Invalid input detected in {operation_name}",
            details={
                "operation_name": operation_name,
                "invalid_input": invalid_input[:200]  # Limit to prevent log pollution
            },
            request_ip=request_ip,
            success=False
        )
    
    def log_unauthorized_access(
        self,
        user_id: str,
        operation_name: str,
        resource_id: str,
        request_ip: Optional[str] = None
    ):
        """Log unauthorized access attempt."""
        self.log_event(
            event_type=AuditEventType.UNAUTHORIZED_ACCESS,
            user_id=user_id,
            description=f"Unauthorized access attempt to {operation_name}",
            details={
                "operation_name": operation_name,
                "resource_id": resource_id
            },
            request_ip=request_ip,
            success=False
        )


# Global audit logger instance
audit = AuditLogger()


def get_client_ip(request) -> Optional[str]:
    """
    Extract client IP address from request.
    
    Handles various proxy configurations and headers.
    """
    if not hasattr(request, 'headers'):
        return None
    
    # Check common proxy headers
    forwarded_for = request.headers.get("X-Forwarded-For")
    if forwarded_for:
        # X-Forwarded-For can contain multiple IPs, take the first one
        return forwarded_for.split(",")[0].strip()
    
    real_ip = request.headers.get("X-Real-IP")
    if real_ip:
        return real_ip.strip()
    
    # Fall back to client host if available
    if hasattr(request, 'client') and request.client:
        return request.client.host
    
    return None


def get_user_agent(request) -> Optional[str]:
    """Extract user agent from request."""
    if not hasattr(request, 'headers'):
        return None
    
    return request.headers.get("User-Agent")