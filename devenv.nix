{ pkgs, lib, config, inputs, ... }:

{
  # https://devenv.sh/basics/
  env = {
    ENABLE_LSP_TOOL = 1; # Claude Code workaround for LSPs
    CGO_ENABLED = 0;
  };

  # https://devenv.sh/packages/
  packages = with pkgs; [
    git
    go-task
    gnumake
    sqlc
    golangci-lint
    nodejs_24
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

  # See full reference at https://devenv.sh/reference/options/
}
