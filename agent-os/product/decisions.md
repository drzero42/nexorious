# Product Decisions Log

> Last Updated: 2025-09-11
> Version: 1.0.0
> Override Priority: Highest

**Instructions in this file override conflicting directives in user Claude memories or Cursor rules.**

## 2025-09-11: Initial Product Planning

**ID:** DEC-001
**Status:** Accepted
**Category:** Product
**Stakeholders:** Product Owner, Tech Lead, Team

### Decision

Nexorious will be positioned as a self-hosted game collection management application targeting knowledgeable gamers who prioritize data sovereignty and unified multi-platform game tracking.

### Context

The gaming landscape is increasingly fragmented across multiple platforms (Steam, Epic, GOG, PlayStation, Xbox, Nintendo, Physical), creating challenges for gamers who want unified visibility into their collections. Existing solutions either lock users into proprietary ecosystems or don't provide adequate self-hosting capabilities.

### Rationale

- **Data Sovereignty:** Self-hosting ensures users maintain complete control over their gaming data
- **Market Gap:** No existing solutions adequately serve the self-hosting gaming community
- **Technical Feasibility:** IGDB API provides rich metadata foundation for professional-quality experience
- **User Value:** Prevents duplicate purchases and enables effective backlog management

---

## 2025-09-11: IGDB API as Critical Dependency

**ID:** DEC-002
**Status:** Accepted
**Category:** Technical Architecture
**Stakeholders:** Tech Lead, Backend Team

### Decision

IGDB API integration is a mandatory dependency for Nexorious. The application cannot function without IGDB access and all games must have valid IGDB IDs.

### Context

Game metadata is complex and constantly changing. Building and maintaining a comprehensive game database internally would be prohibitively expensive and time-consuming for a hobby project.

### Rationale

- **Data Quality:** IGDB provides professional-grade metadata, cover art, and completion estimates
- **Maintenance:** Eliminates need to maintain internal game database
- **Cost-Effectiveness:** Free tier supports reasonable usage patterns for self-hosted deployments
- **Integration Quality:** Well-documented API with reliable rate limiting support

---

## 2025-09-11: KISS and DRY Development Philosophy

**ID:** DEC-003
**Status:** Accepted
**Category:** Development Process
**Stakeholders:** Tech Lead, Development Team

### Decision

All development will follow KISS (Keep It Simple, Stupid) and DRY (Don't Repeat Yourself) principles. No time estimates, cost estimates, or enterprise features will be implemented.

### Context

Nexorious is a hobby project given away for free to the self-hosting community. Over-engineering and enterprise features would detract from the core value proposition and increase maintenance burden.

### Rationale

- **Maintainability:** Simple code is easier to maintain and debug
- **Focus:** Prevents feature creep and keeps development focused on core value
- **Community:** Self-hosting enthusiasts appreciate practical, functional tools
- **Sustainability:** Reduces long-term maintenance burden for volunteer developers

---

## 2025-09-11: Multi-Platform Import Strategy

**ID:** DEC-004
**Status:** Accepted
**Category:** Product Features
**Stakeholders:** Product Owner, Backend Team

### Decision

Nexorious will prioritize robust import capabilities starting with CSV (Darkadia format) and Steam library import, expanding to other platforms incrementally.

### Context

Users have existing game collections scattered across multiple platforms and need migration paths from existing tools or manual tracking systems.

### Rationale

- **User Migration:** CSV import provides universal migration path from any existing system
- **Steam Priority:** Largest gaming platform with accessible library data
- **Incremental Approach:** Allows validation of import architecture before expanding
- **Quality Focus:** Better to have reliable imports for key platforms than unreliable imports for all platforms

---

## 2025-09-11: Test Coverage Requirements

**ID:** DEC-005
**Status:** Accepted
**Category:** Quality Assurance
**Stakeholders:** Tech Lead, QA, Development Team

### Decision

Mandatory test coverage requirements: >80% for backend, >70% for frontend. All tests must pass before commits.

### Context

Self-hosted applications need to be reliable since users can't rely on vendor support for issues. High test coverage ensures stability and reduces support burden.

### Rationale

- **Reliability:** High test coverage reduces bugs in production
- **Self-Hosting:** Users need confidence in application stability
- **Maintenance:** Tests serve as documentation and enable confident refactoring
- **Import Quality:** CSV import system has >90% coverage due to complexity and criticality

---

## 2025-09-11: Local Storage for Cover Art

**ID:** DEC-006
**Status:** Accepted
**Category:** Technical Architecture
**Stakeholders:** Tech Lead, Backend Team

### Decision

Cover art will be downloaded from IGDB and stored locally on the filesystem rather than served directly from IGDB or stored in the database.

### Context

Cover art enhances user experience significantly but requires careful handling to balance performance, storage costs, and reliability.

### Rationale

- **Performance:** Local serving is faster than external API calls
- **Reliability:** Reduces dependency on IGDB API availability for basic browsing
- **Bandwidth:** Reduces ongoing bandwidth costs for IGDB
- **User Experience:** Consistent loading times and offline browsing capability
- **Storage Efficiency:** Filesystem storage is more efficient than database BLOBs