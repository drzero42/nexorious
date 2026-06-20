# nix/nexctl.nix
#
# nexctl is the pure REST client. It builds from the same go.mod/go.sum as the
# nexorious server, so it shares the same vendorHash, supplied by flake.nix
# (goVendorHash) — see that definition for how to refresh it.
{ buildGoModule, lib, stdenv, installShellFiles, src, version, commit, vendorHash }:

buildGoModule {
  pname = "nexctl";
  inherit version src vendorHash;

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

  nativeBuildInputs = [ installShellFiles ];

  # Generate shell completions from the just-built binary. Guarded for
  # cross-builds (running the target binary fails there); our release
  # artifacts build natively per-arch, so this runs in practice.
  postInstall = lib.optionalString (stdenv.buildPlatform.canExecute stdenv.hostPlatform) ''
    installShellCompletion --cmd nexctl \
      --bash <($out/bin/nexctl completion bash) \
      --zsh  <($out/bin/nexctl completion zsh) \
      --fish <($out/bin/nexctl completion fish)
  '';

  meta = {
    description = "Nexorious CLI client";
    homepage = "https://github.com/drzero42/nexorious";
    license = lib.licenses.mit;
    mainProgram = "nexctl";
  };
}
