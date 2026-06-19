# nix/package.nix
#
# Hash update: when go.mod or go.sum changes, set vendorHash = lib.fakeHash,
# run `nix build .#nexorious`, and copy the "got:" hash from the error output.
{ buildGoModule, makeWrapper, postgresql_18, legendary-gl, lib
, src, version, commit, nexorious-frontend }:

buildGoModule {
  pname = "nexorious";
  inherit version src;

  vendorHash = "sha256-CTJYONs70qT60PVIgnSpR8BcKkt/g1TrYOXn8sSrEu0=";

  subPackages = [ "cmd/nexorious" ];

  # Tests require a running Docker daemon (testcontainers-go) which is not
  # available in the Nix sandbox; all tests are covered by CI.
  doCheck = false;

  ldflags = [
    "-s" "-w"
    "-X main.version=${version}"
    "-X main.commit=${commit}"
  ];

  preBuild = ''
    export CGO_ENABLED=0
    # Populate the embed directory with built frontend assets.
    # go:embed in ui/ui.go expects ui/frontend/dist/ to contain real files.
    cp -r ${nexorious-frontend}/. ui/frontend/dist/
  '';

  nativeBuildInputs = [ makeWrapper ];

  postInstall = ''
    wrapProgram $out/bin/nexorious \
      --prefix PATH : ${lib.makeBinPath [ postgresql_18 legendary-gl ]}
  '';

  meta = {
    description = "Nexorious self-hosted game collection manager";
    homepage = "https://github.com/drzero42/nexorious";
    license = lib.licenses.mit;
    mainProgram = "nexorious";
  };
}
