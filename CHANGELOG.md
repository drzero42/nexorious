# Changelog

## [0.3.0](https://github.com/drzero42/nexorious/compare/v0.2.0...v0.3.0) (2026-05-31)


### Features

* move sync schedule into header card ([#710](https://github.com/drzero42/nexorious/issues/710)) ([5868f11](https://github.com/drzero42/nexorious/commit/5868f118d05885730259fb0f604783463005e2eb))


### Bug Fixes

* add SESSION_COOKIE_SECURE config flag (default true) ([#703](https://github.com/drzero42/nexorious/issues/703)) ([1f9bb0f](https://github.com/drzero42/nexorious/commit/1f9bb0f2960e12a00a9578e70a0c9911b7decbb5))
* extract GameFiltersValue type to eliminate duplicate filter shape ([#705](https://github.com/drzero42/nexorious/issues/705)) ([23505a4](https://github.com/drzero42/nexorious/commit/23505a466d6d8274ff54a411ac368442a8efe5dc))
* play_status NULL handling — DB default, sync inference, and filter bug ([#706](https://github.com/drzero42/nexorious/issues/706)) ([#707](https://github.com/drzero42/nexorious/issues/707)) ([d0e064b](https://github.com/drzero42/nexorious/commit/d0e064ba350269a0a7035e144c005690c4c89484))
* set per-route page titles to fix mobile Firefox tab title bug ([#702](https://github.com/drzero42/nexorious/issues/702)) ([36ee7b9](https://github.com/drzero42/nexorious/commit/36ee7b93d8b75577982137188c9e0bd52cbbd13e))
* show Continue button after migration instead of auto-redirecting ([#714](https://github.com/drzero42/nexorious/issues/714)) ([b041eec](https://github.com/drzero42/nexorious/commit/b041eec3d3876349d20ab2f0f46a91ad3041db95))
* show platform name in edit form storefront cards and playtime breakdown ([#712](https://github.com/drzero42/nexorious/issues/712)) ([8f8c094](https://github.com/drzero42/nexorious/commit/8f8c094f63285d725a67812181e93a2b96d95e47))
* **test:** mock document.elementFromPoint in JSDOM setup ([22abe00](https://github.com/drzero42/nexorious/commit/22abe00c3f935efbfabba07b1b85b95f0ad77b1c))

## [0.2.0](https://github.com/drzero42/nexorious/compare/v0.1.3...v0.2.0) (2026-05-31)


### Features

* bumps minor, fix: bumps patch, and feat!: bumps major on the 0.x line. ([01e92a1](https://github.com/drzero42/nexorious/commit/01e92a1fc6b102c0f69eaf4e811bf77292b6dfea))
* clear library (user) and reset database (admin) ([#698](https://github.com/drzero42/nexorious/issues/698)) ([b22c846](https://github.com/drzero42/nexorious/commit/b22c846fd8ce2a95094f381fb1c2052ad5483083))


### Bug Fixes

* **jobs:** count pending_review items regardless of parent job status ([#696](https://github.com/drzero42/nexorious/issues/696)) ([019fdd8](https://github.com/drzero42/nexorious/commit/019fdd87bcd68b147c9d1a023e90cfb519f0192e))
* match platforms by igdb_platform_id in manual add flow ([#690](https://github.com/drzero42/nexorious/issues/690)) ([4e49e6b](https://github.com/drzero42/nexorious/commit/4e49e6b2c2a5f0054bd520cfbf14f4f0112429e9))
* **migrate:** clear lastError in TransitionToReady for structural invariant ([#694](https://github.com/drzero42/nexorious/issues/694)) ([8744451](https://github.com/drzero42/nexorious/commit/87444510a274a75c1696975a58c0e6b1e73a42e6))
* prevent dispatch_sync from permanently deadlocking the sync pipeline ([#692](https://github.com/drzero42/nexorious/issues/692)) ([9d0f2e1](https://github.com/drzero42/nexorious/commit/9d0f2e1ed026342c3d046d6767e77230f5916f88))
* stop caching /api/version and display it correctly in the sidebar ([d1dd682](https://github.com/drzero42/nexorious/commit/d1dd682b7a30d84eca4f558b59d80c41d25f328f))


### Miscellaneous Chores

* correct release version ([f65c220](https://github.com/drzero42/nexorious/commit/f65c2201a94b5ce799a5848358f330510a0d68dd))
* release 0.2.0 ([1de8643](https://github.com/drzero42/nexorious/commit/1de86432b8b1fdff2bdeef94691819a244bfa994))

## [0.1.3](https://github.com/drzero42/nexorious/compare/v0.1.2...v0.1.3) (2026-05-30)


### Features

* add version endpoint and sidebar version display ([#678](https://github.com/drzero42/nexorious/issues/678)) ([e48c052](https://github.com/drzero42/nexorious/commit/e48c0522ec870e516dde0939e3813a98a0f10170))


### Bug Fixes

* handle push failures gracefully in renovate rebase workflow ([#685](https://github.com/drzero42/nexorious/issues/685)) ([5e39761](https://github.com/drzero42/nexorious/commit/5e39761e4fdb967793333ecaddf4e732618e82f2))
* use case-insensitive ~ operator for IGDB exact-name queries ([#686](https://github.com/drzero42/nexorious/issues/686)) ([2b2670b](https://github.com/drzero42/nexorious/commit/2b2670b1768601fbc9d4317908e1508143840803)), closes [#680](https://github.com/drzero42/nexorious/issues/680)
* use persist-credentials false and direct remote set-url for PAT auth ([#684](https://github.com/drzero42/nexorious/issues/684)) ([2aaf546](https://github.com/drzero42/nexorious/commit/2aaf5467f19404ce11c8ef186ff5c90589e5b40c))

## [0.1.2](https://github.com/drzero42/nexorious/compare/v0.1.1...v0.1.2) (2026-05-29)


### Bug Fixes

* correct three bugs in HandleRematchExternalGame ([#673](https://github.com/drzero42/nexorious/issues/673)) ([66a6f7c](https://github.com/drzero42/nexorious/commit/66a6f7c33bfcbf26f61ed94993da14f72ac9a889))

## [0.1.1](https://github.com/drzero42/nexorious/compare/v0.1.0...v0.1.1) (2026-05-29)


### Features

* add release branch CI job and NixOS installation docs ([#666](https://github.com/drzero42/nexorious/issues/666)) ([45cdfb2](https://github.com/drzero42/nexorious/commit/45cdfb2c69f54c91fee13f76e455970dd68ffb9e)), closes [#654](https://github.com/drzero42/nexorious/issues/654)
* **ci:** build and upload release binaries on publish ([#663](https://github.com/drzero42/nexorious/issues/663)) ([8869bd9](https://github.com/drzero42/nexorious/commit/8869bd9d8bd204603145ec98b8b068b99488bbe4))


### Bug Fixes

* **ci:** remove package-name to fix release-please component mismatch ([#660](https://github.com/drzero42/nexorious/issues/660)) ([81e58cb](https://github.com/drzero42/nexorious/commit/81e58cb23def00eb108b8e3279c34a1719a04187))
* **ci:** use PAT for release-please to allow downstream workflow triggers ([#662](https://github.com/drzero42/nexorious/issues/662)) ([21fa2f1](https://github.com/drzero42/nexorious/commit/21fa2f1b8435951ec0760e4069c36c07e2263f17))

## 0.1.0 (2026-05-28)

Initial release.

## Changelog

All notable changes to this project will be documented in this file. The
format and version bumping are managed by [release-please](https://github.com/googleapis/release-please);
do not edit this file by hand.
