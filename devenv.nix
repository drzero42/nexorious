{ pkgs, lib, config, inputs, ... }:

{
  # https://devenv.sh/basics/
  env = {
    ENABLE_LSP_TOOL = 1; # Claude Code workaround for LSPs
    CGO_ENABLED = 0;
    SECRET_KEY = "dev-only-insecure-secret-do-not-use-in-production";
  };

  # https://devenv.sh/packages/
  packages = with pkgs; [
    git
    go-task
    gnumake
    slumber
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

  # https://devenv.sh/tasks/
  tasks = {
    "db:stop" = {
      description = "Stop PostgreSQL without wiping data (workaround for devenv not killing postgres on Ctrl+C)";
      exec = ''
        pg_ctl stop -D "$DEVENV_STATE/postgres" -m fast
        echo "PostgreSQL stopped."
      '';
    };

    "db:reset" = {
      description = "Drop and recreate the nexorious database (cluster keeps running)";
      exec = ''
        dropdb nexorious
        createdb nexorious
        echo "Done. Restart the Go binary to re-run migrations."
      '';
    };

    "db:wipe" = {
      description = "Stop PostgreSQL, delete the entire cluster, and prompt to restart (re-triggers initialDatabases on next devenv up)";
      exec = ''
        pg_ctl stop -D "$DEVENV_STATE/postgres" -m fast 2>/dev/null || true
        rm -rf "$DEVENV_STATE/postgres"
        echo "Cluster wiped. Run 'devenv up' to recreate it."
      '';
    };
  };

  # Podman socket for testcontainers-go integration tests.
  # Ryuk doesn't work with rootless Podman; tests use defer container.Terminate() instead.
  enterShell = ''
    export DOCKER_HOST="unix:///run/user/$(id -u)/podman/podman.sock"
    export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE="/run/user/$(id -u)/podman/podman.sock"
    export TESTCONTAINERS_RYUK_DISABLED="true"
    # Bun pgdriver doesn't inherit PGHOST/PGUSER like libpq — build the full DSN at shell time.
    export DATABASE_URL="postgresql://$USER@/nexorious?host=$PGHOST/.s.PGSQL.5432&sslmode=disable"
  '';

  # See full reference at https://devenv.sh/reference/options/
  dotenv.disableHint = true;
}
