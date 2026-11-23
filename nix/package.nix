# nix/package.nix
#
# The Go vendorHash is supplied by flake.nix (goVendorHash) and shared with
# nix/nexctl.nix — see that definition for how to refresh it.
{ buildGoModule, makeWrapper, installShellFiles, stdenv, postgresql_18, legendary-gl, nile, lib
, src, version, commit, nexorious-frontend, vendorHash }:

let
  # Amazon Games sync needs nile 1.2.0: 1.1.2 (nixos-26.05) keeps its sign-in
  # in a format the connector does not read. Drop the version/src override once
  # the pinned channel ships 1.2.0. The patch is the same one the container
  # image applies (deploy/docker/nile/): unpatched, nile accepts a failed last
  # library page as complete. Keep it until nile fixes that upstream.
  nile_1_2_0 = nile.overridePythonAttrs (old: {
    version = "1.2.0";
    src = old.src.override {
      tag = "v1.2.0";
      hash = "sha256-tzf3sqD7P32AXzZu/WDauOSsEe/xhCh6x4KGQ1YnJqw=";
    };
    patches = (old.patches or [ ]) ++ [ ../deploy/docker/nile/1.2.0-library-status.patch ];
  });
in
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
      --prefix PATH : ${lib.makeBinPath [ postgresql_18 legendary-gl nile_1_2_0 ]}
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
