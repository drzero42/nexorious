# nix/nexctl.nix
#
# nexctl is the pure REST client. It builds from the same go.mod/go.sum as the
# nexorious server, so it shares the same vendorHash — keep this value identical
# to nix/package.nix. Hash update: when go.mod or go.sum changes, set
# vendorHash = lib.fakeHash, run `nix build .#nexctl`, and copy the "got:" hash.
{ buildGoModule, lib, src, version, commit }:

buildGoModule {
  pname = "nexctl";
  inherit version src;

  vendorHash = "sha256-gI4Sw6Eg7GTsjxkdoQUJQIYX0+/8Fk+OPtM7lyH93aE=";

  subPackages = [ "cmd/nexctl" ];

  # Tests require a running Docker daemon (testcontainers-go) which is not
  # available in the Nix sandbox; all tests are covered by CI.
  doCheck = false;

  ldflags = [
    "-s" "-w"
    "-X main.version=${version}"
    "-X main.commit=${commit}"
  ];

  # Pure client: no frontend embed and no PATH wrapping (it shells out to
  # nothing — contrast nix/package.nix).
  preBuild = ''
    export CGO_ENABLED=0
  '';

  meta = {
    description = "Nexorious CLI client";
    homepage = "https://github.com/drzero42/nexorious";
    license = lib.licenses.mit;
    mainProgram = "nexctl";
  };
}
