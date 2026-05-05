{ pkgs, lib, config, inputs, ... }:

{
  # https://devenv.sh/basics/
  env = {
    ENABLE_LSP_TOOL = 1; # Claude Code workaround for LSPs
    CGO_ENABLED = 0;
    DATABASE_URL = "postgresql:///nexorious";
    SECRET_KEY = "dev-only-insecure-secret-do-not-use-in-production";
  };

  # https://devenv.sh/packages/
  packages = with pkgs; [
    git
    go-task
    gnumake
    sqlc
    golangci-lint
    nodejs_24
    uv
  ];

  # https://devenv.sh/languages/
  languages = {
    go = {
      enable = true;
      package = pkgs.go_1_25;
    };
    typescript = {
      enable = true;
    };
  };

  # https://devenv.sh/services/
  services.postgres = {
    enable = true;
    package = pkgs.postgresql_18;
    initialDatabases = [{ name = "nexorious"; }];
  };

  # Podman socket for testcontainers-go integration tests.
  # Ryuk doesn't work with rootless Podman; tests use defer container.Terminate() instead.
  enterShell = ''
    export DOCKER_HOST="unix:///run/user/$(id -u)/podman/podman.sock"
    export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE="/run/user/$(id -u)/podman/podman.sock"
    export TESTCONTAINERS_RYUK_DISABLED="true"
  '';

  # See full reference at https://devenv.sh/reference/options/
}
