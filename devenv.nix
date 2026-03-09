{ pkgs, lib, config, inputs, ... }:

let
  pkgs-upstream = import inputs.nixpkgs-upstream { system = pkgs.stdenv.system; };
in {
  # https://devenv.sh/basics/
  env = {
    # PYTHONPATH = "";
    # UV_CACHE_DIR = "./.uv-cache";
    # LD_LIBRARY_PATH = "${pkgs.stdenv.cc.cc.lib}/lib/:${pkgs.lib.makeLibraryPath [ pkgs.zlib ]}";
    PLAYWRIGHT_BROWSERS_PATH = "${pkgs.playwright.passthru.browsers}";
    ENABLE_LSP_TOOL = 1; # Claude Code workaround for LSPs
  };

  # https://devenv.sh/packages/
  packages = with pkgs; [
    # Git for version control
    git
    # task
    go-task
    # Playwright
    playwright-test
    playwright-mcp
    pkgs-upstream.minikube
    pkgs-upstream.docker-machine-kvm2
  ];

  # https://devenv.sh/languages/
  # languages.typescript.enable = true;

  # https://devenv.sh/processes/
  # processes.dev.exec = "${lib.getExe pkgs.watchexec} -n -- ls -la";

  # https://devenv.sh/services/
  # services.postgres.enable = true;

  # https://devenv.sh/scripts/
  #scripts.hello.exec = ''
  #  echo hello from $GREET
  #'';

  # https://devenv.sh/basics/
  #enterShell = ''
  #  hello         # Run scripts directly
  #  git --version # Use packages
  #'';

  # https://devenv.sh/tasks/
  # tasks = {
  #   "myproj:setup".exec = "mytool build";
  #   "devenv:enterShell".after = [ "myproj:setup" ];
  # };

  # https://devenv.sh/tests/
  #enterTest = ''
  #  echo "Running tests"
  #  git --version | grep --color=auto "${pkgs.git.version}"
  #'';

  # https://devenv.sh/git-hooks/
  # git-hooks.hooks.shellcheck.enable = true;

  # See full reference at https://devenv.sh/reference/options/
}
