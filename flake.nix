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
      _releaseVersion = "0.17.0"; # x-release-please-version
      version =
        if builtins.pathExists ./nix/release-version.txt
        then builtins.readFile ./nix/release-version.txt
        else "main-${self.shortRev or "dirty"}";

      # Single source of truth for the Go vendor hash. The server (nexorious)
      # and client (nexctl) build from the same go.mod/go.sum, and buildGoModule's
      # vendorHash is independent of subPackages, so the two are always identical
      # — sharing one value here makes drift impossible. When go.mod/go.sum
      # changes, the Nix Build workflow rebuilds both packages and patches this
      # line; to refresh by hand set it to lib.fakeHash, run `nix build .#nexorious`,
      # and copy the "got:" hash.
      goVendorHash = "sha256-ml//XaiFFBo7j3w3tSAVIUAq5Pk0jeIXYaZbaNdSGt4=";
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
          vendorHash = goVendorHash;
        };

        # nexctl is the standalone REST client. It is exposed as its own package
        # but is intentionally NOT pulled in by the nexorious NixOS module — the
        # client stays opt-in (add packages.nexctl to environment.systemPackages).
        nexctl = pkgs.callPackage ./nix/nexctl.nix {
          src = self;
          inherit version;
          commit = self.shortRev or "dirty";
          vendorHash = goVendorHash;
        };

        default = nexorious;
      });

      overlays.default = final: _prev: {
        nexorious = self.packages.${final.system}.nexorious;
        nexctl = self.packages.${final.system}.nexctl;
      };

      nixosModules = {
        nexorious = import ./nix/module.nix;
        default = self.nixosModules.nexorious;
      };
    };
}
