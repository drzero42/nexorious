{ pkgs, lib, config, inputs, ... }:

{
  # https://devenv.sh/packages/
  packages = with pkgs; [
    python313
    uv
    # Development tools
    ruff
    # System dependencies for Python packages
    gcc
    pkg-config
    # Common libraries that Python packages might need
    zlib
    openssl
    libffi
  ];

  enterShell = ''
    echo "🐍 Python development environment with uv"
    echo "Python version: $(python --version)"
    echo "uv version: $(uv --version)"
    echo ""
    echo "Available tools:"
    echo "  - uv: Fast Python package manager"
    echo "  - ruff: Python linter and formatter"
    echo "  - playwright: End-to-end testing framework"
    echo ""
    echo "To get started:"
    echo "  uv init --help    # Initialize a new project"
    echo "  uv add <package>  # Add a dependency"
    echo "  uv run <command>  # Run a command in the project environment"
  '';

  # https://devenv.sh/languages/
  languages.python = {
    enable    = true;
    version   = "3.13";
    uv.enable = true;
  };

}
