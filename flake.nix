{
  description = "Development environment for nexorious Python project";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = {
    self,
    nixpkgs,
    flake-utils,
  }:
    flake-utils.lib.eachDefaultSystem (system: let
      pkgs = nixpkgs.legacyPackages.${system};
    in {
      devShells.default = pkgs.mkShell {
        buildInputs = with pkgs; [
          # Python and package management
          python313
          uv

          # Development tools
          ruff
          mypy

          # System dependencies for Python packages
          gcc
          pkg-config

          # Common libraries that Python packages might need
          zlib
          openssl
          libffi

          # Git for version control
          git

          # SQLite for working with DBs
          sqlite

          # Playwright
          playwright-test
        ];

        shellHook = ''
          echo "🐍 Python development environment with uv"
          echo "Python version: $(python --version)"
          echo "uv version: $(uv --version)"
          echo ""
          echo "Available tools:"
          echo "  - uv: Fast Python package manager"
          echo "  - ruff: Python linter and formatter"
          echo "  - mypy: Static type checker"
          echo "  - playwright: End-to-end testing framework"
          echo ""
          echo "To get started:"
          echo "  uv init --help    # Initialize a new project"
          echo "  uv add <package>  # Add a dependency"
          echo "  uv run <command>  # Run a command in the project environment"
        '';

        # Environment variables
        PYTHONPATH = "";
        UV_CACHE_DIR = "./.uv-cache";
        LD_LIBRARY_PATH = "${pkgs.stdenv.cc.cc.lib}/lib/:${pkgs.lib.makeLibraryPath [pkgs.zlib]}";
      };
    });
}
