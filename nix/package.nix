# nix/package.nix
#
# The Go vendorHash is supplied by flake.nix (goVendorHash) and shared with
# nix/nexctl.nix — see that definition for how to refresh it.
{ buildGoModule, makeWrapper, installShellFiles, stdenv, postgresql_18, legendary-gl, lib
, src, version, commit, nexorious-frontend, vendorHash }:

buildGoModule {
  pname = "nexorious";
  inherit version src vendorHash;

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
    # Populate the embedded changelog (go:embed all:data in internal/changelog).
    cp CHANGELOG.md internal/changelog/data/CHANGELOG.md
  '';

  nativeBuildInputs = [ makeWrapper installShellFiles ];

  postInstall = ''
    wrapProgram $out/bin/nexorious \
      --prefix PATH : ${lib.makeBinPath [ postgresql_18 legendary-gl ]}
  '' + lib.optionalString (stdenv.buildPlatform.canExecute stdenv.hostPlatform) ''
    installShellCompletion --cmd nexorious \
      --bash <($out/bin/nexorious completion bash) \
      --zsh  <($out/bin/nexorious completion zsh) \
      --fish <($out/bin/nexorious completion fish)
  '';

  meta = {
    description = "Nexorious self-hosted game collection manager";
    homepage = "https://github.com/drzero42/nexorious";
    license = lib.licenses.mit;
    mainProgram = "nexorious";
  };
}
