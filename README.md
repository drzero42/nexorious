# Nexorious

> [!WARNING]
> **Work in Progress — Not Ready for Use**
> Nexorious is under active development and is not ready for production use or general adoption. Expect breaking changes, missing features, incomplete documentation, and rough edges. Use at your own risk.
>
> **Heads up about version 1.0.0:** The first stable release will be a clean break. When 1.0.0 lands, there will be no automatic upgrade path. To move to it you'll need to export any games you've added, start over with a fresh, empty database, and import your games again. Keep this in mind before you put a lot of data into Nexorious in the meantime.

A self-hosted web application for managing personal video game collections with comprehensive IGDB integration for tracking, organizing, and discovering games across multiple platforms and storefronts.

Nexorious was inspired by [Darkadia](https://darkadia.com) (RIP), a beloved game collection tracker that is no longer operating.

## What Nexorious is — and isn't

Nexorious is a self-hosted catalog and tracker for the games you own across many platforms and storefronts — Steam, Epic, GOG, PlayStation, Xbox, Nintendo, physical media, and more — gathered into one searchable source of truth and enriched automatically with metadata from IGDB. Increasingly, it is also a place to track play status and work through your backlog.

It is **not** a launcher or a library manager. Nexorious never installs, downloads, launches, or plays your games, and it does not touch your save files or game files. If that is what you are after, look at [Playnite](https://playnite.link/), [Heroic](https://heroicgameslauncher.com/), or [Lutris](https://lutris.net/).

## Reasons to use Nexorious

- **Your library is scattered across storefronts.** If you buy games on Steam *and* Epic *and* GOG *and* PlayStation *and* pick up the odd physical copy, Nexorious gives you one consolidated, searchable view of everything you own. This is its core value.
- **Stop buying games you already own.** Before grabbing that bundle or sale, check Nexorious to see whether — and on which storefront — you already own the game.
- **Search, don't type.** Add a game by searching for it — Nexorious pulls everything from IGDB automatically: cover art, descriptions, release dates, genres, ratings, and time-to-beat estimates. Your collection looks complete without manual data entry.
- **Own your data.** Self-hosted, single binary, no third-party cloud, MIT-licensed.
- **Automatic library sync** from Steam, PlayStation Network, GOG, and Epic Games Store today, with more sources on the way.
- **Migrating from Darkadia?** Darkadia (RIP) is gone, but your collection doesn't have to be. Export your Darkadia library to CSV and import it straight into Nexorious — games, platforms, ratings, notes, and the date you added each game all come across.

## Reasons not to use Nexorious

- **You want to launch or play your games.** Nexorious is a catalog, not a launcher — use a launcher like Playnite, Heroic, or Lutris instead.
- **You only use a single storefront.** If everything you own is on Steam (or only PlayStation, or only Xbox), that storefront already shows you your whole library — cross-platform consolidation will not buy you much.
- **You want zero-ops or a hosted service.** There is no SaaS. You run the server and a PostgreSQL database yourself.
- **You're uncomfortable with AI-assisted code.** Nexorious is built extensively with it — see [AI-Assisted Development](#ai-assisted-development).

## Alternatives

- **Hosted web trackers** — [Backloggd](https://www.backloggd.com/), [Grouvee](https://www.grouvee.com/), [Completionator](https://www.completionator.com/), and [HowLongToBeat](https://howlongtobeat.com/) are the closest peers in what they do. They are zero-ops and often social, but they are cloud-hosted rather than self-hosted, and you do not own the data.
- **[Playnite](https://playnite.link/)** — an open-source desktop library aggregator, similar in spirit, but it *is* a launcher and runs only on the Windows desktop rather than as a self-hosted web app.
- **Spreadsheets / Notion** — the DIY default. Total control, but everything is manual: no library sync and no metadata enrichment.

## Features

- **Multi-Platform Game Tracking**: Support for Steam, Epic Games Store, PlayStation, Xbox, Nintendo, GOG, and physical media
- **Sync Integrations**: Automatic library sync from Steam, PlayStation Network (PSN), GOG, and Epic Games Store (via legendary-gl)
- **Rich Game Discovery**: Search and import games from IGDB's extensive database with automatic metadata population
- **Progress Tracking**: Track play status, personal ratings, time played, and detailed notes
- **Import & Export**: Export your collection to JSON or CSV; import from Nexorious's own JSON format or a Darkadia CSV export
- **Easy to self-host**: single Go binary with the React SPA embedded — run the container image, the Helm chart, native `.deb`/`.rpm` packages, or the raw binary
- **Modern Tech Stack**: Go backend with React + Vite frontend

## Documentation

- **[User Guide](docs/user-guide.md)** — using Nexorious day to day: adding and tracking games, finding things in your library, syncing your storefronts, and importing or exporting your collection.
- **[Admin Guide](docs/admin-guide.md)** — running a server: deployment (Docker, Helm, NixOS, single binary), configuration, IGDB setup, user management, backups, maintenance, the command-line tools, and upgrades.
- **[Development Guide](DEV.md)** — for contributors: building from source, the development environment, and the project layout.

## AI-Assisted Development

Nexorious was built with extensive use of AI coding tools. AI assistance was used throughout the project for code generation, architecture decisions, debugging, and documentation.

This is an intentional choice — Nexorious is partly an experiment in what AI-assisted software development can produce. If you have strong objections to AI-generated or AI-assisted code, Nexorious may not be the right project for you.

## Trademarks and Copyright

All mentioned trademarks, brand names, and logos for gaming platforms and storefronts (including but not limited to PlayStation, Xbox, Nintendo, Steam, Epic Games Store, GOG, Apple App Store, Google Play Store, and others) are the property of their respective owners. These trademarks are used solely for identification and compatibility purposes.

The use of these trademarks and brand names does not imply any affiliation, endorsement, or partnership with the respective companies. All rights to these trademarks remain with their original owners.

The logos and icons used in this application are sourced from SVG Repo and other public repositories under various open-source licenses (MIT, CC0, Logo License, etc.).

## License

MIT License — see LICENSE file for details.

---

**Self-hosted game collection management made simple.**
