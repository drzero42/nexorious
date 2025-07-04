# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Python project named "nexorious" that appears to be in its initial setup phase. The repository currently contains only basic project structure files and is licensed under the MIT License.

## Repository Structure

The project is currently minimal with only essential files:
- `README.md` - Basic project information
- `LICENSE` - MIT license
- `.gitignore` - Python-focused gitignore with comprehensive exclusions

## Development Environment

### Nix Development Shell
The project includes a `flake.nix` file that provides a reproducible development environment:
- Run `nix develop` to enter the development shell
- Includes Python 3.13, uv, ruff, mypy, pytest, and system dependencies
- Uses nixpkgs unstable for latest packages

### Python Package Managers
Based on the `.gitignore` file, this project is set up to support multiple Python package managers and tools:
- Standard Python development (pip, setuptools)
- Modern Python tooling (uv, poetry, pdm, pixi)
- Development tools (pytest, mypy, ruff)
- IDE support (PyCharm, VSCode, Cursor)
- Jupyter notebooks and documentation tools

## Current State

This is a fresh repository with no source code yet implemented. The project structure suggests it will be a Python-based application or library, but the specific architecture and build commands are not yet defined.

## Next Steps for Development

When source code is added, this file should be updated to include:
- Build and test commands
- Project structure and architecture details
- Development workflow instructions