# Menu Refactoring Design

## Overview

Simplify the navigation menu by removing unnecessary collapsible sections and relocating items for better accessibility.

## Current State

**Root items:**
- Dashboard
- Library
- Add Game

**Manage section (collapsible):**
- Import / Export
- Sync
- Tags

**Settings section (collapsible):**
- Profile

**Administration section (admin only, collapsible):**
- Admin Dashboard, User Management, Platforms, Maintenance, Backup / Restore

**User dropdown at bottom:**
- Profile
- Log out

## Target State

**Root items:**
- Dashboard
- Library
- Add Game
- Sync *(moved from Manage)*
- Tags *(moved from Manage)*

**Administration section (admin only, collapsible):**
- Admin Dashboard, User Management, Platforms, Maintenance, Backup / Restore

**User dropdown at bottom:**
- Profile
- Import / Export *(moved from Manage)*
- Log out

## Changes Summary

1. **Move Sync to root menu** - Direct access without expanding a section
2. **Move Tags to root menu** - Direct access without expanding a section
3. **Remove Manage section** - No longer needed after relocating items
4. **Remove Settings section** - Profile is already in the user dropdown
5. **Add Import/Export to user dropdown** - Grouped with account/data actions

## Files to Modify

- `frontend/src/components/navigation/nav-items.tsx` - Remove manageSection and settingsSection, add Sync and Tags to mainItems
- `frontend/src/components/navigation/sidebar.tsx` - Remove manageSection and settingsSection rendering, add Import/Export to user dropdown
- `frontend/src/components/navigation/mobile-nav.tsx` - Same changes as sidebar
