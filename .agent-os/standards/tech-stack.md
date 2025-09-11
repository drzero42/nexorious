# Tech Stack

## Context

Global tech stack defaults for Agent OS projects, overridable in project-specific `.agent-os/product/tech-stack.md`.

- Backend App Framework: FastAPI
- Backend Language: Python 3.13+
- Backend Package Manager: uv
- Primary Database: PostgreSQL 17+ or SQLite 3
- ORM: SQLModel
- Frontend JavaScript Framework: Svelte 5
- Frontend Build Tool: Vite
- Frontend Import Strategy: Node.js modules
- Frontend Package Manager: npm
- Node Version: 22 LTS
- CSS Framework: TailwindCSS 4.0+
- Font Provider: Google Fonts
- Font Loading: Self-hosted for performance
- Icons: Lucide React components
- Asset Storage: Staticfiles in FastAPI
- CI/CD Platform: GitHub Actions
- CI/CD Trigger: Push to main/staging branches
- Tests: Run during PR
