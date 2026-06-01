{
  description = "Nexorious — self-hosted game collection manager";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forEachSystem = f: nixpkgs.lib.genAttrs systems
        (system: f nixpkgs.legacyPackages.${system});

      # release-please keeps this in sync with Chart.yaml etc.
      # On the release branch a CI-created nix/release-version.txt overrides it.
      _releaseVersion = "0.5.0"; # x-release-please-version
      version =
        if builtins.pathExists ./nix/release-version.txt
        then builtins.readFile ./nix/release-version.txt
        else "main-${self.shortRev or "dirty"}";
    in
    {
      packages = forEachSystem (pkgs: rec {
        nexorious-frontend = pkgs.callPackage ./nix/frontend.nix {
          src = self;
          inherit version;
        };

        nexorious = pkgs.callPackage ./nix/package.nix {
          inherit nexorious-frontend;
          src = self;
          inherit version;
          commit = self.shortRev or "dirty";
        };

        default = nexorious;
      });

      overlays.default = final: _prev: {
        nexorious = self.packages.${final.system}.nexorious;
      };

      nixosModules = {
        nexorious = import ./nix/module.nix;
        default = self.nixosModules.nexorious;
      };
    };
}
