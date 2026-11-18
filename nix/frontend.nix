# nix/frontend.nix
#
# npm dependencies come from importNpmLock, which fetches each package from the
# `resolved` URL + `integrity` hash already recorded in package-lock.json. There
# is no fixed-output npmDepsHash to keep in sync: a package-lock.json change
# needs no edit here.
{ buildNpmPackage, importNpmLock, lib, src, version }:

let
  # src is the flake root (self); we build the frontend subdirectory.
  npmRoot = "${src}/ui/frontend";
in
buildNpmPackage {
  pname = "nexorious-frontend";
  inherit version;

  src = npmRoot;

  npmDeps = importNpmLock { inherit npmRoot; };
  npmConfigHook = importNpmLock.npmConfigHook;

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
