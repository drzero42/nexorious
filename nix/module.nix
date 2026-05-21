# nix/module.nix
#
# Usage example — add to your NixOS configuration:
#
#   inputs.nexorious.url = "github:drzero42/nexorious";
#
#   { inputs, pkgs, system, ... }: {
#     nixpkgs.overlays = [ inputs.nexorious.overlays.default ];
#     imports = [ inputs.nexorious.nixosModules.default ];
#
#     services.nexorious = {
#       enable = true;
#       environmentFile = "/run/secrets/nexorious.env";
#     };
#   }
#
# The environment file must contain:
#   SECRET_KEY=<random-secret-used-for-JWT-signing-and-encryption>
#   IGDB_CLIENT_ID=<twitch-client-id>
#   IGDB_CLIENT_SECRET=<twitch-client-secret>
#
# Generate SECRET_KEY with: openssl rand -base64 32
# Obtain IGDB credentials at: https://dev.twitch.tv/console
#
# When database.createLocally = true (the default), PostgreSQL is managed
# automatically and no database credentials are needed in the environment file.
# The service connects via the Unix socket using peer authentication.
#
# To use an external PostgreSQL instance, set:
#   database.createLocally = false;
# and include in environmentFile:
#   DATABASE_URL=postgresql://user:password@host:5432/nexorious
{ config, lib, pkgs, ... }:

let
  inherit (lib)
    literalExpression
    mkEnableOption
    mkIf
    mkOption
    mkPackageOption
    optional
    optionalAttrs
    types;

  cfg = config.services.nexorious;
in
{
  options.services.nexorious = {
    enable = mkEnableOption "Nexorious self-hosted game collection manager";

    package = mkPackageOption pkgs "nexorious" { };

    port = mkOption {
      type = types.port;
      default = 8000;
      description = ''
        TCP port Nexorious listens on.
      '';
      example = 3000;
    };

    database = {
      createLocally = mkOption {
        type = types.bool;
        default = true;
        description = ''
          Create and manage a local PostgreSQL database automatically.

          When enabled, `services.postgresql` is configured with the
          `nexorious` user and database, and the service connects via
          the PostgreSQL Unix socket (`/run/postgresql`) using peer
          authentication — no database password is needed in
          `environmentFile`.

          Set to `false` to supply `DATABASE_URL` via `environmentFile`.
        '';
      };

      name = mkOption {
        type = types.str;
        default = "nexorious";
        description = "PostgreSQL database name. Used when `database.createLocally` is `true`.";
      };
    };

    storagePath = mkOption {
      type = types.str;
      default = "/var/lib/nexorious/storage";
      description = ''
        Directory where Nexorious stores uploaded files and internal data.
        Created automatically with ownership `nexorious:nexorious`.
      '';
    };

    backupPath = mkOption {
      type = types.str;
      default = "/var/lib/nexorious/storage/backups";
      description = ''
        Directory where Nexorious writes database backup files.
        Created automatically with ownership `nexorious:nexorious`.
      '';
    };

    workerCount = mkOption {
      type = types.ints.positive;
      default = 4;
      description = ''
        Number of River background job workers.
        Increase for hosts with many concurrent sync jobs.
      '';
      example = 8;
    };

    logLevel = mkOption {
      type = types.enum [ "debug" "info" "warn" "error" ];
      default = "info";
      description = "Log verbosity. Use `debug` to troubleshoot sync or migration issues.";
    };

    environmentFile = mkOption {
      type = types.nullOr types.path;
      default = null;
      description = ''
        Path to a file of `KEY=value` lines loaded into the systemd
        service environment. The file must be readable by the
        `nexorious` system user (mode 0400, owner root is fine — systemd
        reads it before dropping privileges).

        **Required variables:**

        - `SECRET_KEY` — random secret for JWT signing and credential
          encryption. Generate with `openssl rand -base64 32`.
        - `IGDB_CLIENT_ID`, `IGDB_CLIENT_SECRET` — Twitch/IGDB API
          credentials for game metadata enrichment. Obtain at
          <https://dev.twitch.tv/console>.

        When `database.createLocally` is `false`, also add:
        - `DATABASE_URL` — full PostgreSQL connection URL.

        Compatible with sops-nix, agenix, and plain files.
      '';
      example = "/run/secrets/nexorious.env";
    };

    settings = mkOption {
      type = types.attrsOf types.str;
      default = { };
      description = ''
        Additional environment variables merged into the service
        environment. Use as an escape hatch for settings not covered
        by the module options.

        Do not put secrets here — values are world-readable in the Nix
        store. Use `environmentFile` for anything sensitive.
      '';
      example = literalExpression ''
        {
          CORS_ORIGINS = "https://nexorious.example.com";
          METADATA_REFRESH_INTERVAL = "12h";
        }
      '';
    };
  };

  config = mkIf cfg.enable {
    users.users.nexorious = {
      isSystemUser = true;
      group = "nexorious";
      home = "/var/lib/nexorious";
      description = "Nexorious service user";
    };

    users.groups.nexorious = { };

    services.postgresql = mkIf cfg.database.createLocally {
      enable = true;
      package = pkgs.postgresql_18;
      ensureUsers = [
        {
          name = "nexorious";
          ensureDBOwnership = true;
        }
      ];
      ensureDatabases = [ cfg.database.name ];
    };

    systemd.tmpfiles.rules = [
      "d ${cfg.storagePath}              0750 nexorious nexorious -"
      "d ${cfg.backupPath}               0750 nexorious nexorious -"
      "d /var/lib/nexorious/legendary    0750 nexorious nexorious -"
    ];

    systemd.services.nexorious = {
      description = "Nexorious game collection manager";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ]
        ++ optional cfg.database.createLocally "postgresql.service";
      requires = optional cfg.database.createLocally "postgresql.service";

      environment = {
        PORT               = toString cfg.port;
        LOG_LEVEL          = cfg.logLevel;
        STORAGE_PATH       = cfg.storagePath;
        BACKUP_PATH        = cfg.backupPath;
        WORKER_COUNT       = toString cfg.workerCount;
        LEGENDARY_WORK_DIR = "/var/lib/nexorious/legendary";
      }
      // optionalAttrs cfg.database.createLocally {
        DATABASE_URL =
          "postgresql://nexorious@/${cfg.database.name}?host=/run/postgresql&sslmode=disable";
      }
      // cfg.settings;

      serviceConfig = {
        User               = "nexorious";
        Group              = "nexorious";
        StateDirectory     = "nexorious";
        StateDirectoryMode = "0750";
        ExecStart          = "${cfg.package}/bin/nexorious serve";
        Restart            = "on-failure";
        RestartSec         = "5s";
      } // optionalAttrs (cfg.environmentFile != null) {
        EnvironmentFile = cfg.environmentFile;
      };
    };
  };
}
