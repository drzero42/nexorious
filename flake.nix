{
  description = "Nexorious — self-hosted game collection manager";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forEachSystem = f: nixpkgs.lib.genAttrs systems
        (system: f nixpkgs.legacyPackages.${system});

      # Kept in sync with deploy/helm/Chart.yaml by release-please.
      version = "0.0.0"; # x-release-please-version
    in
    {
      packages = forEachSystem (pkgs: rec {
        nexorious-frontend = pkgs.callPackage ./nix/frontend.nix {
          src = self;
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
