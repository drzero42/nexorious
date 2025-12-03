"""
Type-safe wrapper functions for SQLAlchemy column operations.

This module provides typed wrapper functions for SQLAlchemy column methods
that are not recognized by Pyrefly type checker. These wrappers help maintain
type safety while working with SQLAlchemy ORM queries.

Usage:
    from app.utils.sqlalchemy_typed import is_, is_not, ilike, in_, desc, asc, label

    # Instead of: column.is_(None)
    query = query.where(is_(column, None))

    # Instead of: column.ilike(pattern)
    query = query.where(ilike(column, pattern))
"""

from typing import Any
from sqlalchemy import ColumnElement


def is_(column: Any, value: Any) -> ColumnElement[bool]:
    """
    Type-safe wrapper for SQLAlchemy column.is_() method.

    Used for NULL comparisons: column IS NULL or column IS NOT NULL.

    Args:
        column: SQLAlchemy column or expression
        value: Value to compare (typically None for NULL checks)

    Returns:
        SQLAlchemy column element representing the IS comparison

    Example:
        query.where(is_(User.deleted_at, None))  # WHERE deleted_at IS NULL
    """
    return column.is_(value)  # type: ignore[no-any-return]


def is_not(column: Any, value: Any) -> ColumnElement[bool]:
    """
    Type-safe wrapper for SQLAlchemy column.is_not() method.

    Used for negated NULL comparisons: column IS NOT NULL.

    Args:
        column: SQLAlchemy column or expression
        value: Value to compare (typically None for NOT NULL checks)

    Returns:
        SQLAlchemy column element representing the IS NOT comparison

    Example:
        query.where(is_not(User.email, None))  # WHERE email IS NOT NULL
    """
    return column.is_not(value)  # type: ignore[no-any-return]


def ilike(column: Any, pattern: str) -> ColumnElement[bool]:
    """
    Type-safe wrapper for SQLAlchemy column.ilike() method.

    Performs case-insensitive LIKE comparison.

    Args:
        column: SQLAlchemy column or expression (typically text/string column)
        pattern: SQL LIKE pattern with % wildcards

    Returns:
        SQLAlchemy column element representing the ILIKE comparison

    Example:
        query.where(ilike(User.name, '%john%'))  # WHERE name ILIKE '%john%'
    """
    return column.ilike(pattern)  # type: ignore[no-any-return]


def like(column: Any, pattern: str) -> ColumnElement[bool]:
    """
    Type-safe wrapper for SQLAlchemy column.like() method.

    Performs case-sensitive LIKE comparison.

    Args:
        column: SQLAlchemy column or expression (typically text/string column)
        pattern: SQL LIKE pattern with % wildcards

    Returns:
        SQLAlchemy column element representing the LIKE comparison

    Example:
        query.where(like(User.name, '%John%'))  # WHERE name LIKE '%John%'
    """
    return column.like(pattern)  # type: ignore[no-any-return]


def in_(column: Any, values: list[Any]) -> ColumnElement[bool]:
    """
    Type-safe wrapper for SQLAlchemy column.in_() method.

    Checks if column value is in a list of values.

    Args:
        column: SQLAlchemy column or expression
        values: List of values to check against

    Returns:
        SQLAlchemy column element representing the IN comparison

    Example:
        query.where(in_(User.status, ['active', 'pending']))
        # WHERE status IN ('active', 'pending')
    """
    return column.in_(values)  # type: ignore[no-any-return]


def not_in(column: Any, values: list[Any]) -> ColumnElement[bool]:
    """
    Type-safe wrapper for SQLAlchemy column.not_in() method.

    Checks if column value is not in a list of values.

    Args:
        column: SQLAlchemy column or expression
        values: List of values to check against

    Returns:
        SQLAlchemy column element representing the NOT IN comparison

    Example:
        query.where(not_in(User.status, ['deleted', 'banned']))
        # WHERE status NOT IN ('deleted', 'banned')
    """
    return column.not_in(values)  # type: ignore[no-any-return]


def desc(column: Any) -> Any:
    """
    Type-safe wrapper for SQLAlchemy column.desc() method.

    Orders query results by column in descending order.

    Args:
        column: SQLAlchemy column or expression

    Returns:
        SQLAlchemy column element for descending ordering

    Example:
        query.order_by(desc(User.created_at))  # ORDER BY created_at DESC
    """
    return column.desc()  # type: ignore[no-any-return]


def asc(column: Any) -> Any:
    """
    Type-safe wrapper for SQLAlchemy column.asc() method.

    Orders query results by column in ascending order.

    Args:
        column: SQLAlchemy column or expression

    Returns:
        SQLAlchemy column element for ascending ordering

    Example:
        query.order_by(asc(User.name))  # ORDER BY name ASC
    """
    return column.asc()  # type: ignore[no-any-return]


def label(column: Any, name: str) -> Any:
    """
    Type-safe wrapper for SQLAlchemy column.label() method.

    Assigns an alias/label to a column in the query result.

    Args:
        column: SQLAlchemy column or expression
        name: Alias name for the column in results

    Returns:
        SQLAlchemy labeled column element

    Example:
        query.add_columns(label(func.count(User.id), 'user_count'))
        # SELECT COUNT(user.id) AS user_count
    """
    return column.label(name)  # type: ignore[no-any-return]


def contains(column: Any, value: str) -> ColumnElement[bool]:
    """
    Type-safe wrapper for SQLAlchemy column.contains() method.

    Checks if column contains a substring (translates to LIKE '%value%').

    Args:
        column: SQLAlchemy column or expression (typically text/string column)
        value: Substring to search for

    Returns:
        SQLAlchemy column element representing the LIKE comparison

    Example:
        query.where(contains(User.email, '@gmail.com'))
        # WHERE email LIKE '%@gmail.com%'
    """
    return column.contains(value)  # type: ignore[no-any-return]


def startswith(column: Any, value: str) -> ColumnElement[bool]:
    """
    Type-safe wrapper for SQLAlchemy column.startswith() method.

    Checks if column starts with a substring (translates to LIKE 'value%').

    Args:
        column: SQLAlchemy column or expression (typically text/string column)
        value: Prefix to search for

    Returns:
        SQLAlchemy column element representing the LIKE comparison

    Example:
        query.where(startswith(User.username, 'admin'))
        # WHERE username LIKE 'admin%'
    """
    return column.startswith(value)  # type: ignore[no-any-return]


def endswith(column: Any, value: str) -> ColumnElement[bool]:
    """
    Type-safe wrapper for SQLAlchemy column.endswith() method.

    Checks if column ends with a substring (translates to LIKE '%value').

    Args:
        column: SQLAlchemy column or expression (typically text/string column)
        value: Suffix to search for

    Returns:
        SQLAlchemy column element representing the LIKE comparison

    Example:
        query.where(endswith(User.email, '.com'))
        # WHERE email LIKE '%.com'
    """
    return column.endswith(value)  # type: ignore[no-any-return]
