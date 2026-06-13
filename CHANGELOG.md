# Changelog

## [0.13.0](https://github.com/drzero42/nexorious/compare/v0.12.0...v0.13.0) (2026-06-13)


### Features

* bulk pool-membership add endpoint ([#978](https://github.com/drzero42/nexorious/issues/978)) ([5bd4eef](https://github.com/drzero42/nexorious/commit/5bd4eefbe6c45eccc22c7c3b28c8d07919581348)), closes [#975](https://github.com/drzero42/nexorious/issues/975)
* multi-select play status filtering (library + pools) ([#980](https://github.com/drzero42/nexorious/issues/980)) ([a6df879](https://github.com/drzero42/nexorious/commit/a6df87942f8f55ae2df084d833dd3e4a6ee0ff78)), closes [#976](https://github.com/drzero42/nexorious/issues/976)
* per-game pool membership endpoint for Add-to-pool toggle ([#973](https://github.com/drzero42/nexorious/issues/973)) ([f42afef](https://github.com/drzero42/nexorious/commit/f42afefb187d571ae405242a1db8660fd027e9fa)), closes [#971](https://github.com/drzero42/nexorious/issues/971)
* Play Planning backend — pools data model, API, filter primitive, completion hook ([#968](https://github.com/drzero42/nexorious/issues/968)) ([1ed4dfd](https://github.com/drzero42/nexorious/commit/1ed4dfdcebc3bddbc1c5d1e2d750f33d02098fe3)), closes [#955](https://github.com/drzero42/nexorious/issues/955)
* Play Planning frontend — pools page, nav, add-to-pool ([#974](https://github.com/drzero42/nexorious/issues/974)) ([c5a255c](https://github.com/drzero42/nexorious/commit/c5a255cb582a2fe443dca2ff3c274b7db344e2d6)), closes [#956](https://github.com/drzero42/nexorious/issues/956)


### Bug Fixes

* **deps:** update go non-major ([#932](https://github.com/drzero42/nexorious/issues/932)) ([4b99730](https://github.com/drzero42/nexorious/commit/4b99730fd0cd18ff7a72f6796bb823428fb4e7db))
* smooth suggestion → Up Next optimistic move ([#979](https://github.com/drzero42/nexorious/issues/979)) ([effb587](https://github.com/drzero42/nexorious/commit/effb587fc2d76a5e6d83d4930378be9e2b471d8e)), closes [#977](https://github.com/drzero42/nexorious/issues/977)

## [0.12.0](https://github.com/drzero42/nexorious/compare/v0.11.1...v0.12.0) (2026-06-12)


### Features

* add --migrate flag to serve to run migrations on startup ([#950](https://github.com/drzero42/nexorious/issues/950)) ([36c02c4](https://github.com/drzero42/nexorious/commit/36c02c486743dbf82ba74c70f304db3fd3cf0624)), closes [#941](https://github.com/drzero42/nexorious/issues/941)
* add Loki + VictoriaLogs alert rules with opt-in Helm delivery ([#930](https://github.com/drzero42/nexorious/issues/930)) ([f75bf3c](https://github.com/drzero42/nexorious/commit/f75bf3c5e315de3d95f2207c932fcbb005219a9e))
* harden Helm chart for secret rotation and RWO storage ([#933](https://github.com/drzero42/nexorious/issues/933)) ([c10d0a7](https://github.com/drzero42/nexorious/commit/c10d0a7c17eb011de3ca9412f559ee03f5cee6c4))
* metrics-based alert rules (PrometheusRule + VMRule) ([#913](https://github.com/drzero42/nexorious/issues/913)) ([#938](https://github.com/drzero42/nexorious/issues/938)) ([34d04ef](https://github.com/drzero42/nexorious/commit/34d04efa5d033b8fb3ae6d9a74c13bc2586165eb))
* observability deployment — local Grafana stack + Helm ServiceMonitor & dashboard ([#940](https://github.com/drzero42/nexorious/issues/940)) ([8105aa8](https://github.com/drzero42/nexorious/commit/8105aa878f758eb8f19a53a3dc603ce4305d310e)), closes [#912](https://github.com/drzero42/nexorious/issues/912)
* OpenTelemetry metrics foundation + pprof endpoint ([#931](https://github.com/drzero42/nexorious/issues/931)) ([5ac1dcb](https://github.com/drzero42/nexorious/commit/5ac1dcbd2db1088bc8541624dd0bdd4f99ea5f03)), closes [#910](https://github.com/drzero42/nexorious/issues/910)
* opt-in OTLP tracing for the sync pipeline ([#934](https://github.com/drzero42/nexorious/issues/934)) ([3fd94c9](https://github.com/drzero42/nexorious/commit/3fd94c9ca0cb6036783cf72ec645df4ada2a667b))
* sharpen structured logging (correlation ids, leveling, taxonomy) ([#924](https://github.com/drzero42/nexorious/issues/924)) ([42546df](https://github.com/drzero42/nexorious/commit/42546dfdf7757cd7a3bef7c9729ec98740d48afb)), closes [#907](https://github.com/drzero42/nexorious/issues/907)


### Bug Fixes

* close logging gaps blocking log-based alert rules ([#927](https://github.com/drzero42/nexorious/issues/927)) ([4f2d91c](https://github.com/drzero42/nexorious/commit/4f2d91c0f1cd0eb3c9eaadaeb681762eeb26cbff))
* drain in-flight River jobs on graceful shutdown ([#959](https://github.com/drzero42/nexorious/issues/959)) ([bbb3faa](https://github.com/drzero42/nexorious/commit/bbb3faa6111823a74854469a63e937c49ca562f6)), closes [#947](https://github.com/drzero42/nexorious/issues/947)
* harden Helm/deploy observability schema and guards ([#958](https://github.com/drzero42/nexorious/issues/958)) ([06240ae](https://github.com/drzero42/nexorious/commit/06240aea581aa1ac122f658ca103ab1117feda80)), closes [#946](https://github.com/drzero42/nexorious/issues/946)
* harden HTTP surface and finish structured-logging sweep ([#957](https://github.com/drzero42/nexorious/issues/957)) ([2ec50ab](https://github.com/drzero42/nexorious/commit/2ec50ab338b94df81b2c73bf9b1bfe9a6ecc92cb)), closes [#945](https://github.com/drzero42/nexorious/issues/945)
* make alert rules fire and record hard-failed syncs in metrics ([#953](https://github.com/drzero42/nexorious/issues/953)) ([716536c](https://github.com/drzero42/nexorious/commit/716536c0e0a58faa3b6ba676345244fc8987c276)), closes [#944](https://github.com/drzero42/nexorious/issues/944)
* make TestLoad_ObservabilityDefaults hermetic against ambient OTel/pprof env ([#948](https://github.com/drzero42/nexorious/issues/948)) ([8bc997f](https://github.com/drzero42/nexorious/commit/8bc997f6b25896c1fc564db4d8c4e62fc05a40f8)), closes [#936](https://github.com/drzero42/nexorious/issues/936)
* make the logging pipeline see panic and River boundaries ([#952](https://github.com/drzero42/nexorious/issues/952)) ([adae3a5](https://github.com/drzero42/nexorious/commit/adae3a535ff29094ce434290e6e601eb461ce03d)), closes [#943](https://github.com/drzero42/nexorious/issues/943)
* scrub credential-bearing URL queries from logs and persisted errors ([#949](https://github.com/drzero42/nexorious/issues/949)) ([2943879](https://github.com/drzero42/nexorious/commit/2943879c4062d26eb54102089972b50985e3351c)), closes [#937](https://github.com/drzero42/nexorious/issues/937)
* tag retryInsert unsupported job_type log with validation category ([#929](https://github.com/drzero42/nexorious/issues/929)) ([3dbd990](https://github.com/drzero42/nexorious/commit/3dbd99012ae0f88c7959aa8306def99c421c2cf2)), closes [#928](https://github.com/drzero42/nexorious/issues/928)

## [0.11.1](https://github.com/drzero42/nexorious/compare/v0.11.0...v0.11.1) (2026-06-10)


### Bug Fixes

* make container image binary executable ([#922](https://github.com/drzero42/nexorious/issues/922)) ([1ceff88](https://github.com/drzero42/nexorious/commit/1ceff88b913ca8870c321a6b9f6756880394592d))

## [0.11.0](https://github.com/drzero42/nexorious/compare/v0.10.1...v0.11.0) (2026-06-10)


### Features

* native .deb/.rpm packages and release-only build pipeline ([#917](https://github.com/drzero42/nexorious/issues/917)) ([afa7179](https://github.com/drzero42/nexorious/commit/afa717946fb694a27c874929ec8a40e45da5c19b)), closes [#901](https://github.com/drzero42/nexorious/issues/901)
* notify when a newer version is available ([#914](https://github.com/drzero42/nexorious/issues/914)) ([a0c1c47](https://github.com/drzero42/nexorious/commit/a0c1c47b345bf0079e4d42051a3691b5d81880ba)), closes [#899](https://github.com/drzero42/nexorious/issues/899)
* show available updates in the version subcommand ([#916](https://github.com/drzero42/nexorious/issues/916)) ([307157e](https://github.com/drzero42/nexorious/commit/307157ec415ff22f820661aea940464fdb6ba574))


### Bug Fixes

* remove dead tag description field ([#895](https://github.com/drzero42/nexorious/issues/895)) ([#905](https://github.com/drzero42/nexorious/issues/905)) ([2148e8d](https://github.com/drzero42/nexorious/commit/2148e8d3228162c0b26205d08a58cd75e9c16923))

## [0.10.1](https://github.com/drzero42/nexorious/compare/v0.10.0...v0.10.1) (2026-06-09)


### Bug Fixes

* embed only user and admin guides in-app ([#902](https://github.com/drzero42/nexorious/issues/902)) ([aa03b95](https://github.com/drzero42/nexorious/commit/aa03b95bc38b795e5aca688a6bd9bc6bc7984158))
* include embedded docs in container build context ([#904](https://github.com/drzero42/nexorious/issues/904)) ([1cd607a](https://github.com/drzero42/nexorious/commit/1cd607a010ebb92bfb6e7a0000100e5be016c6c6))

## [0.10.0](https://github.com/drzero42/nexorious/compare/v0.9.0...v0.10.0) (2026-06-09)


### Features

* render docs/ guides in-app (embedded markdown viewer) ([#900](https://github.com/drzero42/nexorious/issues/900)) ([f7433f9](https://github.com/drzero42/nexorious/commit/f7433f9b35210db8be7d37d8db417bd1846b1d28)), closes [#887](https://github.com/drzero42/nexorious/issues/887)


### Bug Fixes

* clamp howlongtobeat values to NUMERIC(6,2) column max ([#882](https://github.com/drzero42/nexorious/issues/882)) ([e533dff](https://github.com/drzero42/nexorious/commit/e533dffe13258ce41fe7406f8668ab1f2f6042e5)), closes [#869](https://github.com/drzero42/nexorious/issues/869)
* default is_available to true in HandleCreatePlatform ([#886](https://github.com/drzero42/nexorious/issues/886)) ([ac671e0](https://github.com/drzero42/nexorious/commit/ac671e000f7ee470312ab98edf276a2410f9ae07)), closes [#880](https://github.com/drzero42/nexorious/issues/880)
* detect Epic DLC via metadata.mainGameItem ([#885](https://github.com/drzero42/nexorious/issues/885)) ([461ea86](https://github.com/drzero42/nexorious/commit/461ea863ff331c01f2a70e0d2d6d9e4be41358ad)), closes [#870](https://github.com/drzero42/nexorious/issues/870)
* maintenance start handlers create jobs row and return real job_id ([#892](https://github.com/drzero42/nexorious/issues/892)) ([6ea2c09](https://github.com/drzero42/nexorious/commit/6ea2c09ed5db62dd614daf4eec6b5c34ab708198)), closes [#890](https://github.com/drzero42/nexorious/issues/890)
* prevent stale completed maintenance job from re-appearing on start ([#889](https://github.com/drzero42/nexorious/issues/889)) ([a64a9dd](https://github.com/drzero42/nexorious/commit/a64a9dd5c0ac05fac71bb1db8fcd0f00430da38d)), closes [#884](https://github.com/drzero42/nexorious/issues/884)
* serialize job creation with advisory locks to close TOCTOU duplicate-job races ([#894](https://github.com/drzero42/nexorious/issues/894)) ([ecbe152](https://github.com/drzero42/nexorious/commit/ecbe152bb29b9934aee4fa8a0fa3eeeac6ef9213)), closes [#891](https://github.com/drzero42/nexorious/issues/891)

## [0.9.0](https://github.com/drzero42/nexorious/compare/v0.8.1...v0.9.0) (2026-06-09)


### Features

* deep-link to each storefront's product page from the game details page ([#871](https://github.com/drzero42/nexorious/issues/871)) ([bc1a8bf](https://github.com/drzero42/nexorious/commit/bc1a8bf572a5347361bc574858a94eb24ef0f4e0)), closes [#831](https://github.com/drzero42/nexorious/issues/831)
* link Recent Activity games to their library entry ([#825](https://github.com/drzero42/nexorious/issues/825)) ([#866](https://github.com/drzero42/nexorious/issues/866)) ([e4d91b1](https://github.com/drzero42/nexorious/commit/e4d91b180e87f5eb9c31cb2272306687fcf5e364))
* search IGDB games by bare numeric ID (no `igdb:` prefix) ([#873](https://github.com/drzero42/nexorious/issues/873)) ([e3bd6ca](https://github.com/drzero42/nexorious/commit/e3bd6caea870769d386fe296d0c79434edbb2c3f)), closes [#843](https://github.com/drzero42/nexorious/issues/843)
* set per-platform ownership/hours/acquired in the add-game wizard ([#876](https://github.com/drzero42/nexorious/issues/876)) ([3ca760e](https://github.com/drzero42/nexorious/commit/3ca760efa70c525179a1c28e19a2bf8eb5d48bbb)), closes [#858](https://github.com/drzero42/nexorious/issues/858)
* unify sync-source slugs with storefronts.name (remove epic/psn ↔ catalog mapping) ([#863](https://github.com/drzero42/nexorious/issues/863)) ([b239802](https://github.com/drzero42/nexorious/commit/b2398023917610af287ee8311f216eed48e3cff5))
* wishlist for games the user wants but doesn't own ([#879](https://github.com/drzero42/nexorious/issues/879)) ([e9a6c8c](https://github.com/drzero42/nexorious/commit/e9a6c8c00997995e483c9173e97d2388c9f18cf4)), closes [#867](https://github.com/drzero42/nexorious/issues/867)


### Bug Fixes

* allow adding duplicate platforms with different storefronts ([#848](https://github.com/drzero42/nexorious/issues/848)) ([#857](https://github.com/drzero42/nexorious/issues/857)) ([f21222d](https://github.com/drzero42/nexorious/commit/f21222da0be070515886fd1fd7706e52241aa165))
* **deps:** update go non-major ([#855](https://github.com/drzero42/nexorious/issues/855)) ([b36a0da](https://github.com/drzero42/nexorious/commit/b36a0da556126e9437f2a6ea00cc4fd9dea152e7))
* Humble Bundle connection card link and Enter-to-submit ([#868](https://github.com/drzero42/nexorious/issues/868)) ([#872](https://github.com/drzero42/nexorious/issues/872)) ([a1d55a9](https://github.com/drzero42/nexorious/commit/a1d55a99469589695846612eee5cea07b69324c6))
* identify edit-page platform rows by identity, not name ([#846](https://github.com/drzero42/nexorious/issues/846), [#847](https://github.com/drzero42/nexorious/issues/847)) ([#851](https://github.com/drzero42/nexorious/issues/851)) ([9e5691f](https://github.com/drzero42/nexorious/commit/9e5691f39855ca6aea440f9b0884efc668a136f9))
* name the storefront in the sync.diff "library changes" notification ([#874](https://github.com/drzero42/nexorious/issues/874)) ([35074e4](https://github.com/drzero42/nexorious/commit/35074e4aae6e695651ebbdbe3b762f772735c36a)), closes [#844](https://github.com/drzero42/nexorious/issues/844)
* persist and display per-platform acquired date ([#861](https://github.com/drzero42/nexorious/issues/861)) ([7fb6f23](https://github.com/drzero42/nexorious/commit/7fb6f232ca99b5f6770681425a315309b59615f9)), closes [#849](https://github.com/drzero42/nexorious/issues/849)
* reflect clicked star rating immediately on edit page ([#862](https://github.com/drzero42/nexorious/issues/862)) ([28cde10](https://github.com/drzero42/nexorious/commit/28cde1011481b152e75959d7e7078705e03e2d92))
* surface games already in library on Add Game search ([#856](https://github.com/drzero42/nexorious/issues/856)) ([#859](https://github.com/drzero42/nexorious/issues/859)) ([1cf5d25](https://github.com/drzero42/nexorious/commit/1cf5d25a876f6d7595ced71b3cb4e81da03e5ab3))

## [0.8.1](https://github.com/drzero42/nexorious/compare/v0.8.0...v0.8.1) (2026-06-05)


### Bug Fixes

* accept Darkadia CSV exports with extra feature-toggle columns ([#839](https://github.com/drzero42/nexorious/issues/839)) ([d613a5a](https://github.com/drzero42/nexorious/commit/d613a5a9ae4421c28b960e7a9fc35ea18668bda4))

## [0.8.0](https://github.com/drzero42/nexorious/compare/v0.7.0...v0.8.0) (2026-06-05)


### Features

* auto-dismiss import/export progress box on clean completion ([#833](https://github.com/drzero42/nexorious/issues/833)) ([f4157ef](https://github.com/drzero42/nexorious/commit/f4157efbad3cbd4cbe7fa4a2b0aee27087067a99))
* bring JSON import/export into compliance with v2.0 interchange spec ([#836](https://github.com/drzero42/nexorious/issues/836)) ([11b02d8](https://github.com/drzero42/nexorious/commit/11b02d81fe8d4c4d790884febf072c01965544a3))
* Darkadia CSV import ([#824](https://github.com/drzero42/nexorious/issues/824)) ([cec4547](https://github.com/drzero42/nexorious/commit/cec4547af85fbfaf22d344e2c899422031ac8b49))
* expand manual-workflow reference data ([#818](https://github.com/drzero42/nexorious/issues/818)) ([#829](https://github.com/drzero42/nexorious/issues/829)) ([d59107a](https://github.com/drzero42/nexorious/commit/d59107acfa976ee5ec8e9f8fb8a6c4f290efaf7b))
* **import:** drop Item Details box, add Retry Failed to progress actions ([#755](https://github.com/drzero42/nexorious/issues/755)) ([#811](https://github.com/drzero42/nexorious/issues/811)) ([660374d](https://github.com/drzero42/nexorious/commit/660374d3626568aac85a92f2a43059e53be2ff86))
* **notify:** notify on storefront credential expiry ([#751](https://github.com/drzero42/nexorious/issues/751)) ([#815](https://github.com/drzero42/nexorious/issues/815)) ([d5684d5](https://github.com/drzero42/nexorious/commit/d5684d54d90d22253a1bc2fdd4c0573bc97089d1))
* **sync:** add Humble Bundle sync source ([#766](https://github.com/drzero42/nexorious/issues/766)) ([#819](https://github.com/drzero42/nexorious/issues/819)) ([ebde66c](https://github.com/drzero42/nexorious/commit/ebde66ca44dbcc440bd975f3a640d995181e1f6c))


### Bug Fixes

* **admin:** give the activity date-range filter a visible label ([#816](https://github.com/drzero42/nexorious/issues/816)) ([1b01477](https://github.com/drzero42/nexorious/commit/1b01477c5bd21e212cbcb0bde7b2f9074ad6bfa4))
* **admin:** guard inverted date range in activity filter ([#812](https://github.com/drzero42/nexorious/issues/812)) ([8ed9d2f](https://github.com/drzero42/nexorious/commit/8ed9d2f55850782248b973171f273ee6d009930d))
* **api:** escape ILIKE wildcards in user-supplied filters ([#750](https://github.com/drzero42/nexorious/issues/750)) ([#814](https://github.com/drzero42/nexorious/issues/814)) ([ec0dfd3](https://github.com/drzero42/nexorious/commit/ec0dfd37833bbb36c859eed61548e0b69d4cecfb))
* interpret admin activity date filter in local timezone ([#795](https://github.com/drzero42/nexorious/issues/795)) ([be04f86](https://github.com/drzero42/nexorious/commit/be04f86f5ade4248576c54d7eb4c6f52f7f3c56b))
* make platform/tag selector dropdown lists scroll instead of clipping ([#834](https://github.com/drzero42/nexorious/issues/834)) ([2b3bcaa](https://github.com/drzero42/nexorious/commit/2b3bcaab8c83e617a94573fd08ee6fbaaa9b2334))
* **sync:** drop opaque account-ID line and fix Epic blank "Connected as" ([#802](https://github.com/drzero42/nexorious/issues/802)) ([8a6bde0](https://github.com/drzero42/nexorious/commit/8a6bde0475cf98c9810c3b10cffbccf26380e1a3))
* **sync:** prune dead account-ID fields from connection/status GET responses ([#803](https://github.com/drzero42/nexorious/issues/803)) ([d623da0](https://github.com/drzero42/nexorious/commit/d623da03b812be03edf15de8f0c87a26ad605ab2))
* **sync:** prune dead PSN response fields (region, is_verified) ([#806](https://github.com/drzero42/nexorious/issues/806)) ([a28c9f0](https://github.com/drzero42/nexorious/commit/a28c9f0fb3d26c779f3d7e74e5898b6dff939e79))
* **sync:** stop mirroring Steam credentials into preferences.steam ([#797](https://github.com/drzero42/nexorious/issues/797)) ([c82587a](https://github.com/drzero42/nexorious/commit/c82587acc3e0e255f200e1aed0eb36e4c93ff248))
* **sync:** stop storing dead access_token / user_id in GOG credentials blob ([#808](https://github.com/drzero42/nexorious/issues/808)) ([3616765](https://github.com/drzero42/nexorious/commit/361676502da7e37d02511c48fcb6bda66075c369))
* validate play_status against enum in Darkadia import path ([#837](https://github.com/drzero42/nexorious/issues/837)) ([a5aaccf](https://github.com/drzero42/nexorious/commit/a5aaccfe38f044e618cee39d910d857f28a05e81)), closes [#835](https://github.com/drzero42/nexorious/issues/835)

## [0.7.0](https://github.com/drzero42/nexorious/compare/v0.6.0...v0.7.0) (2026-06-03)


### Features

* **notify:** typed payload contract with handled decode errors ([#791](https://github.com/drzero42/nexorious/issues/791)) ([33269f4](https://github.com/drzero42/nexorious/commit/33269f4018d38c752217da45a98eff13283e051f))
* unify Recent Activity into one component over a generic changes table ([#754](https://github.com/drzero42/nexorious/issues/754)) ([74fe7be](https://github.com/drzero42/nexorious/commit/74fe7bec79db1bd561f9610688370e3c6a0ac442))


### Bug Fixes

* enable gosec and close restore path-traversal / decompression-bomb gaps ([#784](https://github.com/drzero42/nexorious/issues/784)) ([32a7ebe](https://github.com/drzero42/nexorious/commit/32a7ebee2f163befdc4da0a71c31212f6e465e9c)), closes [#781](https://github.com/drzero42/nexorious/issues/781)
* PSN sync imports only owned games, play history sets playtime only ([#776](https://github.com/drzero42/nexorious/issues/776)) ([aa9c597](https://github.com/drzero42/nexorious/commit/aa9c5979fe7b1e8901a323324ffddb5769c27d27))
* redirect running SPA on backend app-state change ([#771](https://github.com/drzero42/nexorious/issues/771)) ([#773](https://github.com/drzero42/nexorious/issues/773)) ([ef8dfb9](https://github.com/drzero42/nexorious/commit/ef8dfb9f9fdb9593b2058ad132eeee56f89a30ec))
* stop dropping Steam Mac platform during sync ([#770](https://github.com/drzero42/nexorious/issues/770)) ([858e837](https://github.com/drzero42/nexorious/commit/858e8374a48365363e1e039eb2eeee5f5fc39695))

## [0.6.0](https://github.com/drzero42/nexorious/compare/v0.5.0...v0.6.0) (2026-06-02)


### Features

* accept full GOG redirect URL when connecting sync ([#743](https://github.com/drzero42/nexorious/issues/743)) ([347fe2d](https://github.com/drzero42/nexorious/commit/347fe2d71ac2ae9328d21816b2180d0c94ef9616))
* add admin activity/events view ([#747](https://github.com/drzero42/nexorious/issues/747)) ([dc8b7bb](https://github.com/drzero42/nexorious/commit/dc8b7bb2d1c2246bb3ac31497f7e79354b63ab4b))
* add reset-password CLI command ([#744](https://github.com/drzero42/nexorious/issues/744)) ([2fd5b75](https://github.com/drzero42/nexorious/commit/2fd5b75bcee2805222e56326a371ae2030a0ffb0))
* add setup CLI command to create the first admin user ([#745](https://github.com/drzero42/nexorious/issues/745)) ([9c14b31](https://github.com/drzero42/nexorious/commit/9c14b31cd167388bcef09fde0be2420ec5c5b250))
* add web UI for managing API keys ([#746](https://github.com/drzero42/nexorious/issues/746)) ([9b53b19](https://github.com/drzero42/nexorious/commit/9b53b198db457666f97e4e3d4522a37f6269f6f3)), closes [#732](https://github.com/drzero42/nexorious/issues/732)
* print help instead of starting server on bare invocation ([#741](https://github.com/drzero42/nexorious/issues/741)) ([18ac3b4](https://github.com/drzero42/nexorious/commit/18ac3b44ea87f34eeb04e16e4cb9a7eea37a8521))


### Bug Fixes

* **deps:** update react monorepo ([#739](https://github.com/drzero42/nexorious/issues/739)) ([ed11dd3](https://github.com/drzero42/nexorious/commit/ed11dd3188089a5c456d22401197d2b169d467b0))

## [0.5.0](https://github.com/drzero42/nexorious/compare/v0.4.0...v0.5.0) (2026-06-01)


### Features

* add notifications ([#738](https://github.com/drzero42/nexorious/issues/738)) ([9dea81c](https://github.com/drzero42/nexorious/commit/9dea81cffc2735c5e9d9b8f03e586e2d5db782bd))
* CLI api-key subcommand for managing API keys ([#626](https://github.com/drzero42/nexorious/issues/626)) ([#733](https://github.com/drzero42/nexorious/issues/733)) ([104e8aa](https://github.com/drzero42/nexorious/commit/104e8aa27d12d8d7a240c76fd130f96100cc618b))
* CLI login/logout/whoami for API-key bootstrap ([#723](https://github.com/drzero42/nexorious/issues/723)) ([4bfbf2d](https://github.com/drzero42/nexorious/commit/4bfbf2d34e6eaba14b4f34971481ea061e4d24f5))
* unify import/export and sync job tracking ([#670](https://github.com/drzero42/nexorious/issues/670)) ([#722](https://github.com/drzero42/nexorious/issues/722)) ([cfe1108](https://github.com/drzero42/nexorious/commit/cfe11080e62b7d1d5b4a9835ac15c6bd75b76848))

## [0.4.0](https://github.com/drzero42/nexorious/compare/v0.3.1...v0.4.0) (2026-06-01)


### Features

* auto-promote play_status to in_progress when hours added ([#725](https://github.com/drzero42/nexorious/issues/725)) ([768db37](https://github.com/drzero42/nexorious/commit/768db37f3d89677db001940bab197327936c387d))

## [0.3.1](https://github.com/drzero42/nexorious/compare/v0.3.0...v0.3.1) (2026-06-01)


### Bug Fixes

* emit full socket path in createLocally DATABASE_URL ([#721](https://github.com/drzero42/nexorious/issues/721)) ([88cedc7](https://github.com/drzero42/nexorious/commit/88cedc77c0fd2d5934b8b935a057f33050767e9e)), closes [#720](https://github.com/drzero42/nexorious/issues/720)
* keep Continue button disabled when migration verification fails ([#715](https://github.com/drzero42/nexorious/issues/715)) ([dd51183](https://github.com/drzero42/nexorious/commit/dd51183a53bc98de96818fdcb4e61a37d80d88e9))
* route single-value URL filters through getOne helper ([#718](https://github.com/drzero42/nexorious/issues/718)) ([6494032](https://github.com/drzero42/nexorious/commit/6494032a319ea3c68926a6c974ae191d27d012a6)), closes [#649](https://github.com/drzero42/nexorious/issues/649)
* use background ctx for dispatch_complete gate write ([#719](https://github.com/drzero42/nexorious/issues/719)) ([01c16dc](https://github.com/drzero42/nexorious/commit/01c16dc07c7291de4b428c3e2376b5a459b0bc09)), closes [#699](https://github.com/drzero42/nexorious/issues/699)

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
