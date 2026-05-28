# Changelog

## 0.1.0 (2026-05-28)


### ⚠ BREAKING CHANGES

* replace JWT auth with server-side sessions and API keys ([#656](https://github.com/drzero42/nexorious/issues/656))

### Features

* add loved filter to games library ([#651](https://github.com/drzero42/nexorious/issues/651)) ([ad9243c](https://github.com/drzero42/nexorious/commit/ad9243c70e6a3fdb1d5552702f4c57c49ac1b2d3))
* **branding:** add brand icon for SPA, static pages, and Helm chart ([#576](https://github.com/drzero42/nexorious/issues/576)) ([5781885](https://github.com/drzero42/nexorious/commit/5781885152eeb70723c9567a9ced8264b5835358))
* encrypt storefront credentials at rest (issue [#599](https://github.com/drzero42/nexorious/issues/599)) ([#605](https://github.com/drzero42/nexorious/issues/605)) ([76bf897](https://github.com/drzero42/nexorious/commit/76bf8976625f86ec18c42f87274a6fe1d1b928b9))
* filter IGDB search by platform during sync matching ([#615](https://github.com/drzero42/nexorious/issues/615)) ([#616](https://github.com/drzero42/nexorious/issues/616)) ([2389431](https://github.com/drzero42/nexorious/commit/2389431539f56239a994fdf627d9543947922b89))
* half-hour-granular playtime — capture Steam/PSN sub-hour precision and clean up hours display ([#645](https://github.com/drzero42/nexorious/issues/645)) ([e99a528](https://github.com/drzero42/nexorious/commit/e99a52854f3267296a6ce24dc643e9127ff703c4))
* **helm:** external-secret postgres creds, auto-gen password, storageClass ([54941cb](https://github.com/drzero42/nexorious/commit/54941cbc05446929f775ea438cdd5394ac76739a))
* immediately fetch IGDB metadata for newly synced games ([#630](https://github.com/drzero42/nexorious/issues/630)) ([c08f537](https://github.com/drzero42/nexorious/commit/c08f537322b412516f49cd9c8c4037b535228971))
* move import/export into main navigation menu ([#628](https://github.com/drzero42/nexorious/issues/628)) ([44fd0f4](https://github.com/drzero42/nexorious/commit/44fd0f466be43e6384d839a61b8bbaf3de929a48))
* **nix:** add package derivation and NixOS module ([#592](https://github.com/drzero42/nexorious/issues/592)) ([b0fe8b3](https://github.com/drzero42/nexorious/commit/b0fe8b335edfd86db5c00a42cb4547784e75760c))
* replace JWT auth with server-side sessions and API keys ([#656](https://github.com/drzero42/nexorious/issues/656)) ([76a1e52](https://github.com/drzero42/nexorious/commit/76a1e520bb1b7c5347a4a51dc854dcddfa6e45fd))
* **setup:** restore from on-disk backups during initial setup ([#581](https://github.com/drzero42/nexorious/issues/581)) ([447e3da](https://github.com/drzero42/nexorious/commit/447e3daa6418333eff822f651e4851c9274522ff))
* **steam:** multi-platform sync via appdetails, GOG Mac parity ([#526](https://github.com/drzero42/nexorious/issues/526)) ([#578](https://github.com/drzero42/nexorious/issues/578)) ([bb91a19](https://github.com/drzero42/nexorious/commit/bb91a19106b0d3196bb7ec39bc83dd3861387ff6))


### Bug Fixes

* accept howlongtobeat_main and rating_average user-games sorts ([#648](https://github.com/drzero42/nexorious/issues/648)) ([bcb7a31](https://github.com/drzero42/nexorious/commit/bcb7a31891fcd674979734fd7ee69ec4c11de210)), closes [#639](https://github.com/drzero42/nexorious/issues/639)
* add debug logging to GOG adapter ([bc73990](https://github.com/drzero42/nexorious/commit/bc739907f4b08ebd3788599d8a63acf4e7d0c25d))
* calculate user-game hours_played from platform hours ([#640](https://github.com/drzero42/nexorious/issues/640)) ([4beb49e](https://github.com/drzero42/nexorious/commit/4beb49e251ed18e13715c11bfa58f9bf16c8b444))
* **ci:** pin inaugural release to 0.1.0 and trim its changelog ([#572](https://github.com/drzero42/nexorious/issues/572)) ([d2822e3](https://github.com/drzero42/nexorious/commit/d2822e3eb73cd5d9022293f857472e8ac7463c70))
* **deps:** update module golang.org/x/crypto to v0.52.0 ([#596](https://github.com/drzero42/nexorious/issues/596)) ([3e81f0d](https://github.com/drzero42/nexorious/commit/3e81f0dba3114653026390d41458d5bf53b741c0))
* **deps:** update river monorepo to v0.38.0 ([#603](https://github.com/drzero42/nexorious/issues/603)) ([f0c3910](https://github.com/drzero42/nexorious/commit/f0c391046b556ea02632bced326821b145488a53))
* enable errcheck check-blank gate and surface swallowed sync writes ([#587](https://github.com/drzero42/nexorious/issues/587)) ([#631](https://github.com/drzero42/nexorious/issues/631)) ([f373ab3](https://github.com/drzero42/nexorious/commit/f373ab370f87eda90025d3848dcb89931ff8241c))
* **frontend:** sync tokensRef on refresh to avoid 401 retry race ([00eba61](https://github.com/drzero42/nexorious/commit/00eba6110507ee96dc2e3a5e856c8589f9673461))
* **frontend:** widen JobPriority to match backend domain ([#579](https://github.com/drzero42/nexorious/issues/579)) ([43d6984](https://github.com/drzero42/nexorious/commit/43d69848dea07451c23bd461b755cf0f277008f8))
* **helm:** add dbEncryptionKey fields to values schema ([6f2f232](https://github.com/drzero42/nexorious/commit/6f2f23291861678962bedabba3635d0bebb13bc5))
* **helm:** mount /tmp emptyDir on main container ([2c085d6](https://github.com/drzero42/nexorious/commit/2c085d6b4e575e30d4cf819b8d28eae05315b931))
* **helm:** repair common 5.0.1 dependency drift; tidy chart docs ([#571](https://github.com/drzero42/nexorious/issues/571)) ([e156969](https://github.com/drzero42/nexorious/commit/e15696923cb0b38685b10ba774697e665791420c))
* **helm:** valid helm.sh/chart label, skip empty credentials Secret ([8302fc7](https://github.com/drzero42/nexorious/commit/8302fc7c793c3f538377fa0b642913eca90ec1e4))
* **igdb:** retry transparently on 429 and cap burst to req/s ([#573](https://github.com/drzero42/nexorious/issues/573)) ([92a26ab](https://github.com/drzero42/nexorious/commit/92a26abab5d7c52ce7e26895da8a135927620312))
* log + surface DB and job-queue write failures (issue [#534](https://github.com/drzero42/nexorious/issues/534) sev 3) ([#585](https://github.com/drzero42/nexorious/issues/585)) ([#593](https://github.com/drzero42/nexorious/issues/593)) ([2e9f6db](https://github.com/drzero42/nexorious/commit/2e9f6dbe43d945b1c8d0ac26c3340ee30ef917c8))
* **migrate:** emit SSE log line on synchronous bunMig.Lock failure ([#637](https://github.com/drzero42/nexorious/issues/637)) ([5a38a41](https://github.com/drzero42/nexorious/commit/5a38a4133c73ce83aec7a77815d857d8b5235519))
* **migrate:** surface migration failures via app state ([#583](https://github.com/drzero42/nexorious/issues/583)) ([#588](https://github.com/drzero42/nexorious/issues/588)) ([66c08ed](https://github.com/drzero42/nexorious/commit/66c08ed325c478445ec8ef515b5453e595a47fb1))
* prevent sync jobs from completing while batches are still enqueuing ([#644](https://github.com/drzero42/nexorious/issues/644)) ([8193c2a](https://github.com/drzero42/nexorious/commit/8193c2a6775d16ff71632bb2d51577046d20dd7e))
* remove dead manual-match UI (resolve/skip) from JobItemsDetails ([#617](https://github.com/drzero42/nexorious/issues/617)) ([#624](https://github.com/drzero42/nexorious/issues/624)) ([a1ce51b](https://github.com/drzero42/nexorious/commit/a1ce51b1bf8523cf62dd9c3fd71a98a70fa060ec))
* **steam:** fix sync timing out after 60s for large libraries ([#609](https://github.com/drzero42/nexorious/issues/609)) ([ab36f68](https://github.com/drzero42/nexorious/commit/ab36f68f8a43337a43ae0073f3ce23ecc25cad85)), closes [#606](https://github.com/drzero42/nexorious/issues/606)
* stop silently producing wrong data (issue [#534](https://github.com/drzero42/nexorious/issues/534) sev 4) ([#586](https://github.com/drzero42/nexorious/issues/586)) ([#601](https://github.com/drzero42/nexorious/issues/601)) ([f813009](https://github.com/drzero42/nexorious/commit/f8130091910ef091bedb76b5005b0ceb0dbdf6a5))
* stop swallowing auth and credential errors (issue [#534](https://github.com/drzero42/nexorious/issues/534) sev 2) ([#591](https://github.com/drzero42/nexorious/issues/591)) ([5aba5df](https://github.com/drzero42/nexorious/commit/5aba5dfdb152e14dc359a4544ec24dc219188943))
* surface backend error-key detail in API toasts ([#638](https://github.com/drzero42/nexorious/issues/638)) ([7f64277](https://github.com/drzero42/nexorious/commit/7f6427713519ab6ecd98df7fcfa1f24fdd8b9565))
* **sync:** prevent job_items stuck in pending after igdb_failed auto-retry ([#574](https://github.com/drzero42/nexorious/issues/574)) ([fefbcf8](https://github.com/drzero42/nexorious/commit/fefbcf8d6f8134d3bad14b711d58ad9f51e89c0a))
* **sync:** Recent Activity stale after last needs-review match completes job ([#621](https://github.com/drzero42/nexorious/issues/621)) ([5ec242a](https://github.com/drzero42/nexorious/commit/5ec242adb97487636cefac7ec95514c90026883e))
* **sync:** reconcile sync activity totals — add skipped and already_in_library buckets ([#629](https://github.com/drzero42/nexorious/issues/629)) ([8e3cb2a](https://github.com/drzero42/nexorious/commit/8e3cb2a71ece8bcc16f966baf322ec7515fc75cd))

## Changelog

All notable changes to this project will be documented in this file. The
format and version bumping are managed by [release-please](https://github.com/googleapis/release-please);
do not edit this file by hand.
