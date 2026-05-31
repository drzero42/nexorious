# nix/frontend.nix
#
# Hash update: when package-lock.json changes, regenerate npmDepsHash with:
#   nix run nixpkgs#prefetch-npm-deps -- ui/frontend/package-lock.json
{ buildNpmPackage, lib, src, version }:

buildNpmPackage {
  pname = "nexorious-frontend";
  inherit version;

  # src is the flake root (self); we reference the frontend subdirectory.
  src = "${src}/ui/frontend";

  npmDepsHash = "sha256-/64ERx3o95JUV3p3t3Ndo5lyayWuvhuutSiS0s+ioeE=";

  installPhase = ''
    runHook preInstall
    cp -r dist $out
    runHook postInstall
  '';

  meta = {
    description = "Nexorious frontend assets (React SPA)";
    license = lib.licenses.mit;
  };
}
