{ pkgs, lib, config, inputs, ... }:

{
  # https://devenv.sh/basics/
  env = {
    ENABLE_LSP_TOOL = 0; # Disable LSP for Claude Code
    CGO_ENABLED = 0;
    DB_ENCRYPTION_KEY = "dev-only-insecure-db-key-do-not-use-in-production";
  };

  # https://devenv.sh/packages/
  packages = with pkgs; [
    git
    gnumake
    go-task
    golangci-lint
    imagemagick
    jq
    inputs.drzero42.packages.${system}.slumber
    legendary-gl
    librsvg
    nodejs_24
    procps
    uv
    yamllint
  ];

  # https://devenv.sh/languages/
  languages = {
    go = {
      enable = true;
      package = pkgs.go_1_26;
    };
    nix = {
      enable = true;
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

  # https://devenv.sh/git-hooks/
  # The full test suites run at `git push` (pre-push) so quick commits stay
  # fast; the lighter format/lint/build checks are handled by Claude Code hooks
  # (.claude/hooks/). These install/refresh whenever the devenv shell is
  # re-entered, and each is scoped to run only when its files are in the push.
  git-hooks.hooks = {
    go-lint = {
      enable = true;
      name = "golangci-lint run";
      entry = "golangci-lint run";
      types = [ "go" ];
      pass_filenames = false;
      stages = [ "pre-push" ];
    };
    go-test = {
      enable = true;
      name = "go test ./...";
      entry = "go test -timeout 600s ./...";
      types = [ "go" ];
      pass_filenames = false;
      stages = [ "pre-push" ];
    };
    frontend-check = {
      enable = true;
      name = "frontend check + knip + test";
      entry = "bash -c 'cd ui/frontend && npm run check && npm run knip && npm run test'";
      files = "^ui/frontend/";
      pass_filenames = false;
      stages = [ "pre-push" ];
    };
  };

  # https://devenv.sh/tasks/
  tasks = {
    "db:stop" = {
      description = "Stop PostgreSQL without wiping data (workaround for devenv not killing postgres on Ctrl+C)";
      exec = ''
        PG_DATA="$DEVENV_STATE/postgres"
        if pg_ctl stop -D "$PG_DATA" -m fast 2>/dev/null; then
          echo "PostgreSQL stopped."
        else
          # Ctrl+C can leave postgres fully orphaned (no PID file, no socket).
          # The postmaster is always the oldest (lowest PID) postgres process,
          # since it starts before forking its workers.
          PG_PID=$(pgrep -o postgres 2>/dev/null)
          if [ -n "$PG_PID" ]; then
            kill -INT "$PG_PID"
            while kill -0 "$PG_PID" 2>/dev/null; do sleep 0.1; done
            echo "PostgreSQL stopped (via signal)."
          else
            echo "Could not find postgres process." >&2
            exit 1
          fi
        fi
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

  enterShell = ''
    # Bun pgdriver doesn't inherit PGHOST/PGUSER like libpq — build the full DSN at shell time.
    export DATABASE_URL="postgresql://$USER@/nexorious?host=$PGHOST/.s.PGSQL.5432&sslmode=disable"
    export LEGENDARY_WORK_DIR="$DEVENV_ROOT/.legendary-work"
  '';

  # See full reference at https://devenv.sh/reference/options/
  dotenv.disableHint = true;
}
