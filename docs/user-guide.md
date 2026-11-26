# Nexorious User Guide

This guide is for anyone using Nexorious to keep track of their game collection. It walks through what you can do with the app and where to do it: cataloguing the games you own, tracking how far you've got with them, syncing your storefront libraries, and moving your data in and out.

It assumes someone has already set up and is running a Nexorious server, and that you have an account on it. If you're the one running the server, see the [Admin Guide](admin-guide.md) for deployment, configuration, and operations.

> Nexorious is under active development. Pages and details described here may change between versions, and you should expect the occasional rough edge.

## Getting Started

Nexorious is built around a simple idea: a **library** of the games you own and a **wishlist** of the ones you want. You fill both either by hand, a game at a time, or in bulk by **syncing** a storefront account — and whatever you add is enriched automatically with cover art and details from IGDB, so you rarely type more than a title.

If you've just got an account, here's your first ten minutes:

1. **Sign in** with your username and password.
2. **Choose how to fill your library.** If your collection already lives on Steam, GOG, Epic, PlayStation, Humble Bundle, or Amazon Games, go to **Sync** and connect an account — for most people that's the quickest way to get going, since it pulls your whole purchase history in at once. If you only want to track a handful of titles, use **Add Game** to search for them instead. You can do both, and add more anytime.
3. **Review any sync matches.** A sync brings most games in automatically; the few Nexorious isn't sure about wait in a **Pending Review** list for you to confirm. Clearing it once leaves your library tidy.
4. **Explore your Library.** Browse, search, and filter your collection, and start tracking where you are with each game using its **play status**.

The rest of this guide is reference detail — read the parts you need when you need them.

## Signing in and getting around

Open your Nexorious server in a browser and sign in with the username and password you were given (or that you created when first setting up the server). Sessions last a while, so you usually won't have to sign in again every visit.

Once you're in, everything is reachable from the sidebar on the left (it collapses into a menu on narrow screens):

- **Dashboard** — a landing page with a summary of your collection and recent activity.
- **Library** — your full collection, where you search, filter, and sort.
- **Wishlist** — games you want but don't own yet.
- **Add Game** — search IGDB and add a game to your library or wishlist.
- **Sync** — connect your Steam, PlayStation, GOG, Epic, Humble Bundle, and Amazon accounts so your purchases flow in automatically.
- **Tags** — your own labels for organising games.
- **Import / Export** — bring a collection in from a file, or take yours out.

Your username sits at the top, with links to your **Profile** and to log out.

If you ever see a banner saying IGDB isn't configured or its credentials are invalid, search and metadata enrichment won't work until whoever runs the server fixes it — that's an admin task, covered in the Admin Guide.

## Your account

Open **Profile** from the user menu to manage your own account.

- **Username** — you can change it. The page checks as you type whether the new name is free. Changing it signs you out, so you'll log back in with the new name.
- **Password** — change it by entering your current password and a new one (at least 8 characters). A strength indicator gives you a rough sense of how good the new password is. Changing your password also signs you out.
- **Deal region** — sets the region used for the price and deal links shown on wishlisted games, so the prices you see match where you actually buy.
- **API keys** — create keys here if you want to talk to Nexorious from a script or the command-line tool. Each key has a name, a scope, and an optional expiry. A **read** key can only read your data — any attempt to create, change, or delete is rejected — which makes it safe to hand to a script or third-party tool that only needs to look. (It can still search IGDB, which only reads, and connect an [AI agent](#connecting-ai-chats-and-agents) that gets only the read tools.) A **write** key has full access. A key's full value is shown only once when you create it, so copy it then. You can revoke a key at any time.
- **Notifications** — set up where you want to be notified and what about (see [Notifications](#notifications) below).

At the bottom is a **Clear Library** action. It removes every game from your collection in one go and can't be undone, so it asks you to type a confirmation first. It clears your games — it doesn't delete your account.

## Adding games

Go to **Add Game** and search for a title. Results come from IGDB, so you get the canonical name, cover art, and details without typing them yourself. Each result tells you whether the game is already in your library or wishlist, so you don't add duplicates.

If the title results don't surface the right game, you can search by **IGDB ID** instead. To find a game's ID, go to igdb.com and search for it there; open its page, and you'll find an **"IGDB ID: nnnnn"** field listing the number. Copy that number (not the page URL) and paste it into the Nexorious search box. A bare number looks the game up by ID *and* searches by title, so a game whose name is itself a number still turns up; prefix it with `igdb:` (for example `igdb:1020`) to match only that exact ID. The same trick works in `nexctl` — just pass the ID to `game add`.

Pick a game and you'll see a preview — cover, developer, release date, the platforms IGDB knows about, and rough time-to-beat estimates. From here you:

1. Choose whether it goes to your **library** (a game you own) or your **wishlist** (one you want).
2. Pick the **platforms** you have it on. You can select more than one, and optionally note the storefront for each (for example Steam, GOG, or a physical copy).
3. Optionally fill in ownership details there and then — how you own it, when you got it, and hours played — or leave that for later.

Add it, and it lands in your collection. If a game genuinely isn't in IGDB you won't be able to add it by search; in practice almost everything is there.

## Viewing and editing a game

Click any game to open its page. You'll see the cover, the metadata pulled from IGDB (publisher, genres, release date, game modes, and so on), a link to its IGDB page, your play status and rating, and a row for each platform you own it on — including any direct links to the game's page on the storefronts you bought it from.

Press **Edit** to change your own details:

- **Play status** — where you are with the game (see below).
- **Rating** — your personal score, one to five stars.
- **Loved** — mark a game as a favourite.
- **Copies** — add or remove copies of the game. Each copy is one platform, and optionally the storefront you got it from; for each one set how you own it (owned, borrowed, rented, via a subscription, or no longer owned), the date you acquired it, and hours played. Hours for a synced Steam game are filled in from Steam and can't be edited by hand.
- **Tags** — attach your own labels, creating new ones inline as you go.
- **Notes** — free-form notes, with basic formatting.

Total hours played is shown as the sum across all your copies, so you don't add it up yourself.

To remove a game entirely, use the **Remove** button on its page.

## Tracking your progress

Nexorious is as much about working through your backlog as cataloguing it. The main tracking tools are:

- **Play status** — where you are with a game. Use whichever values make sense to you; the eight options mean:
  - *Not Started* — owned but not yet started.
  - *In Progress* — currently playing (also where a replay goes once you actually start it).
  - *Completed* — finished the main story.
  - *Mastered* — finished the main story **and all side quests**.
  - *Dominated* — Mastered **and** earned every trophy/achievement (100%).
  - *Shelved* — on hold, but you intend to return to it later.
  - *Dropped* — abandoned, no longer playing.
  - *Replay* — a game you *want* to replay; move it to *In Progress* when you actually start the replay.
- **Rating** — your own one-to-five-star score, separate from the IGDB community rating shown alongside it.
- **Hours played** — tracked per platform, and summed for you.
- **Loved** — a simple favourite flag you can filter on.
- **Notes** — anything you want to remember about a game.

## Finding games in your library

The **Library** page is built for digging through a large collection. At the top you have a search box (by title), a sort control with a direction toggle, and a switch between grid and list views.

You can filter by play status, ownership status, platform, and whether a game is loved, and open a "more filters" section for storefront, genre, game mode, theme, player perspective, and tags. Combine as many as you like; a clear-filters control resets them. Large collections are paged, and you can choose how many games to show at once.

Selecting several games at once lets you act on them together — change their play status or remove them in bulk — which saves a lot of clicking when tidying up.

## Library health

**Library Health** runs a set of checks over your collection and lists the games whose data looks wrong or out of date. Nothing is changed until you say so. **Refresh** runs the checks again.

The checks come in two tiers:

- **Inconsistencies** — data that is wrong and probably needs fixing. The Library Health entry in the sidebar shows a badge with how many games these checks flag.
- **Smells** — data that looks out of date or incomplete. It might be worth updating, but it isn't wrong.

| Check | Tier | What it flags | Auto-fix |
|---|---|---|---|
| Storefront-less platform | Inconsistency | A copy that doesn't say which storefront it's from | — |
| Orphan game | Inconsistency | A game in your library with no copies | — |
| Wishlisted yet owned | Inconsistency | A game still on your wishlist that is already in your library | Takes it off the wishlist |
| Missing ownership status | Inconsistency | A copy that doesn't say whether you own it, borrowed it, and so on | — |
| Impossible acquired date | Inconsistency | A date you got the game that is in the future, or before the game was released | — |
| Invalid storefront for platform | Inconsistency | A copy whose platform and storefront don't go together, like a PlayStation game on Steam | — |
| Played but "not started" | Smell | A game marked **Not Started** that has playtime | Sets it to **In Progress** |
| In progress but never touched | Smell | A game marked **In Progress** with no playtime | Sets it to **Not Started** |
| Unrated after beating | Smell | A game you beat (**Completed**, **Mastered** or **Dominated**) that has no rating | — |

Open a check to see the games it flags. For a check with an auto-fix, **Apply** fixes one game, and **Apply to all** fixes every game the check flags. A check without one leaves the fix to you: open the game and edit it.

If a flagged game is how you want it, **Ignore** it. That check stops flagging the game; the other checks still look at it. **Show ignored** lists the games you've ignored for a check, and **Restore** makes the check flag a game again.

The same works from the command line with [`nexctl doctor`](#the-everyday-commands), and from an AI chat or agent through the `library_smells_*` tools (see [Connecting AI chats and agents](#connecting-ai-chats-and-agents)).

## Wishlist

The **Wishlist** holds games you want but don't own. Add a game to it the same way you add to your library — just choose "wishlist" as the destination on the add screen.

A wishlisted game's page shows price-and-deal links (for PC and for console) based on the deal region set in your profile, so you can check current prices at a glance. When you do buy it, open the game and use **Move to library**, pick the platform(s) you got it on, and it moves across. Games also leave your wishlist automatically if they turn up in a storefront sync — once you own it, it's no longer something to want.

## Tags

**Tags** are your own labels — "co-op night," "to finish in 2026," whatever suits you. On the Tags page you create them with a name and a colour, and you can see how many games use each one. Edit or delete a tag at any time; deleting it just removes the label, not the games. You attach tags to a game from its edit page.

## Planning

A library tells you what you *have*; the **Planning** page helps you decide what to play *next*. It's built around **pools** — named collections of games you're planning to play, that you can put in whatever order you like.

How you organise them is up to you. Keep one running "up next" list, or split your plans into as many pools as suit you — one per play partner ("co-op with Sam"), one per mood or genre ("RPGs," "quick shooters"), whatever fits how you decide. The same game can sit in several pools at once, and a pool can hold games you don't own yet alongside ones you do, so a pool is as much a plan for what to buy and play as for what to play next.

Create a pool from the Planning page — give it a name and, if you like, a colour — and drag your pools there to set the order they're listed in.

### A pool, and its queue

Open a pool and you get three stacked sections:

- **Queue** — the ordered list of games you plan to play. Drag cards to reorder them; the game at the top is marked **Play Next**, your on-deck pick. A pool you never reorder is just a flat list; a pool you do reorder is a proper queue. Either way it's the same thing — a queue is only a pool with an order you care about.
- **Candidates** — games you've earmarked for the pool but haven't placed in the queue yet. Promote a candidate into the **Queue** when you're ready to commit to an order, or demote one back out if you change your mind.
- **Suggestions** — games from your library and wishlist that match the pool's filter but aren't in the pool yet, each with an **Add** button. This is the "matches this pool — add?" list.

### Filling a pool

You can add a game to a pool from three places: the **Add** button in a pool's Suggestions list, the **Add to pool** action on a game's own page, and the bulk **Add to pool** control on the [Library](#finding-games-in-your-library) page once you've selected several games at once. The first is for working through what a pool's filter turns up; the other two are for dropping in a specific game wherever you happen to be looking.

### The pool filter

Each pool can have one saved filter — **Add filter** (or **Edit filter**) at the top of the pool. It drives the Suggestions list: owned and wishlisted games matching it, minus whatever's already in the pool. The filter speaks the same facets as library search — play status, genre, theme, platform, storefront, tags, rating, game mode, perspective, and time-to-beat — and you can stack several filter cards, where a game counts if it matches *any* of them.

Suggestions are simply this filter applied to your collection — nothing more. There's no hidden ranking or "for you" guesswork: a game shows up because it matches the filter you set, so the suggestions are entirely yours to shape. One thing worth knowing is that a game with no genre or theme recorded can't match a filter built on those, so the more complete your metadata, the more useful the suggestions. Finished games are left out automatically (see below), so you're never pointed at something you've already played.

### Wishlist games in a pool

A pool can include games you don't own yet, not just ones you do. A game you've [wishlisted](#wishlist) shows a **Buy first** badge instead of the usual play controls, so you can tell at a glance which games you'd need to buy before playing. When you get one — either by [moving it to your library](#wishlist) yourself or through a storefront sync — it turns into a normal owned game and keeps its place in the pool and queue, so you don't have to add it again.

### When you finish a game

When you set a game's [play status](#tracking-your-progress) to **Completed**, **Mastered**, **Dominated**, or **Dropped**, it's automatically removed from every pool and queue it belongs to, and the queue moves on to the next game. This keeps your pools focused on what's still ahead of you, rather than games you're already done with. If you want to play one again later, just add it back.

## Syncing your storefront libraries

Sync is what makes Nexorious worth running if your games are spread across several stores: connect an account once and your purchases there show up in your library automatically, enriched with IGDB metadata. Each person connects their own accounts — your credentials are yours and are stored encrypted on the server.

The **Sync** page shows a card for each supported storefront — **Steam**, **Epic Games Store**, **PlayStation Store**, **GOG**, **Humble Bundle**, and **Amazon Games** — with its connection status, when it last synced, and how many games are waiting for your attention.

### Connecting a storefront

Open a storefront to connect it. What you provide depends on the store:

- **Steam** — your Steam ID and a Steam Web API key. The page links to where you get each.
- **PlayStation Network** — an NPSSO token from your signed-in PSN session; the page links to instructions for retrieving it.
- **GOG** and **Epic Games Store** — open the login page Nexorious gives you, sign in at the store, then paste the authorization code (or the redirect URL you land on) back into Nexorious.
- **Humble Bundle** — sign in to bring across your Humble purchases.
- **Amazon Games** — choose **Sign in with Amazon**, open the Amazon sign-in page it links to, sign in, then paste the full address of the amazon.com page you land on back into Nexorious within 15 minutes. This brings across your Amazon Games purchases and Prime Gaming claims.

Once connected, the card shows your account on that storefront and a way to disconnect.

### Sync schedule and running it manually

For each storefront you can choose how often it syncs — manually only, or hourly, daily, or weekly. Whatever the schedule, **Sync Now** runs it on demand. The storefront's page shows the last sync time and the progress of a running one. If a scheduled sync fails, Nexorious tries again after 15 minutes, then waits twice as long after each further failure, but never longer than the schedule itself. You get one **Sync failed** notification when a storefront starts failing, not one per attempt; the next failure after a successful sync notifies again. When a storefront that sent **Sync failed** syncs successfully again, you get a **Sync working again** notification.

### Reviewing matches

Most games match to IGDB automatically. The ones Nexorious isn't sure about land in a **Pending Review** list on the storefront's page, and the Sync entry in the sidebar shows a badge with how many are waiting. For each one you can **find the right match** from IGDB search, or **skip** it so it's left out and not raised again. That search box accepts an **IGDB ID** as well as a title — handy when the title results miss the mark (see [Adding games](#adding-games) for how to find the ID). Matched games can be **re-matched** if Nexorious got one wrong, and anything you skipped can be unskipped later. A game you unskip before it was ever matched appears under **Unmatched**, where you can find its match or skip it again; the next sync also tries to match it while it is still in your storefront library. Matching, re-matching and unskipping take effect in your library straight away. Games whose sync failed appear separately with the reason, and you can retry them individually or all at once.

A sync won't be treated as fully done while items still need review, so it's worth clearing the list now and then.

#### Games with more than one edition

Some stores list a game more than once: PlayStation Store has separate PS4 and PS5 versions, for example. Copies of a game with the same title on the same store, matched to the same game (or skipped together), are its **editions**. They show as one entry, and matching, skipping or unskipping it applies to all of them.

When an entry has more than one edition, you can pick one edition instead. The match dialog, **Skip** and **Unskip** then ask whether to act on **All editions** (the default) or a single edition, each listed with its platforms, the store's ID for it and a link to its store page. In the Matched list, entries with more than one edition also have a **Skip** button for this.

- **Two different games with the same title.** If two different games share a title on one store, Nexorious first treats them as one game. Open the entry, choose the edition that is the other game, and match it to the right game. It becomes its own entry with its own library game. Entries that share a title show their platforms and store ID under the title, so you can tell them apart.
- **One edition you don't want.** If a store sells a game for several platforms in one package and you don't care about one of them, skip that edition alone. It moves to Skipped as its own entry and the others stay as they are. Skipping doesn't remove anything from your library. If that platform is already on the game in your library, remove it there.
- **Putting them back together.** Match the edition back to the same game as the others, or unskip it, and it joins their entry again.

One edition stays separate only while it has a match or a skip of its own. A copy with neither takes on the decision of the oldest copy with the same title the next time it's synced. That decision might be the one you just made. So decide every edition you mean to keep separate. An edition you unskip alone that has no match is skipped again at the next sync. The dialogs warn you when that can happen.

### When credentials expire

Store sessions don't last forever. When a storefront's credentials stop working, its card shows a credentials error and you'll need to reconnect it — re-enter the token or run the sign-in flow again. You can also have Nexorious notify you when this happens (see below).

## Importing and exporting

The **Import / Export** page moves whole collections in and out.

**Exporting** gives you two formats. JSON is the complete picture — every game with its platforms, tags, notes, and ratings — and is the one to use for a backup or to move to another Nexorious instance. CSV is a flatter, spreadsheet-friendly summary. Both download as a dated file.

**Importing** comes in a few flavours. Every one of them matches your games to IGDB, so each needs IGDB configured on the server, and some games may need your review afterwards — just like a storefront sync; you'll find those on the Import / Export page.

- **Nexorious JSON or CSV** — restore or merge a collection from a Nexorious export. Either format merges into what you already have rather than wiping it.
- **vglist JSON** — a migration path for anyone coming from [vglist](https://vglist.co). In vglist, go to **Settings → Export Library** to download your library as a JSON file, then import that file here. Your ratings, play status, playtime, and which stores you own each game on are brought across; the storefront and platform are matched up as closely as vglist's data allows, with anything that doesn't map cleanly kept as a note on the game.
- **CSV from another tracker** — bring in a library from a CSV export. Exports from known trackers are **recognised automatically** when you pick the file, and their preset maps them across with no setup: **Darkadia**, **Grouvee**, **Completionator**, and Nexorious's own CSV export. You can also pick a preset yourself from the **Preset** list. For anything else, choose your file and map its columns to Nexorious fields yourself in the dialog that follows — so any tracker that can export a CSV can come in. As with the other formats, values that don't map cleanly are kept as a note rather than dropped.

These migration imports are one-off — they bring a collection in once, and don't keep syncing afterwards. All imports run in the background, so you can watch their progress and see recent runs listed on the page.

## Why sync and import take a while

Bringing in a large library isn't instant, and that's by design rather than a sign something's wrong. Every game Nexorious pulls in — from a storefront sync or an import alike — is looked up in IGDB to attach its cover art, description, release date, and the rest of its metadata. IGDB and the stores themselves each accept only a handful of requests per second, so Nexorious paces its work to stay comfortably within those limits and be a well-behaved guest. Pushing harder would risk being throttled or temporarily blocked, which would make everything slower in the end.

So a first sync or import of hundreds of games can take a while to finish, and there's nothing on your side to fix. It all runs in the background — carry on using your library and watch the progress on the **Sync** or **Import / Export** page. Later syncs are usually much quicker, since only new purchases need looking up.

## Using nexctl (the command-line client)

Everything in this guide is reachable from a browser, but Nexorious also ships a command-line client, **`nexctl`**, that talks to the same server over its API. It's a separate program from the server itself: cataloguing games, managing pools and tags, running syncs, and importing or exporting all work from the terminal — handy for scripting, bulk edits, or driving Nexorious from a tool or an AI agent. The full surface is larger than what's shown here; run `nexctl --help`, or `nexctl <command> --help` for any group, to see all of it.

### Getting nexctl

`nexctl` is published with each release as its own download — a raw binary and `.deb` / `.rpm` packages for Linux. It also ships inside the server container image at `/usr/local/bin/nexctl`. To install it on a separate machine, see the [Admin Guide](admin-guide.md) for packaging details, or grab it from the project's releases.

### Signing in

Authenticate once and `nexctl` remembers it:

```bash
nexctl account login
```

It prompts for your server URL, username, and password, then stores an API key locally (under `~/.config/nexorious/`, or `$XDG_CONFIG_HOME`). After that, commands just work. The key can change your data; add `--scope read` for a profile that can only look. `nexctl account whoami` shows who you're signed in as, and `nexctl account logout` clears the stored key.

If you'd rather not log in interactively — for a script, a cron job, or a third-party tool — create an **API key** instead, either from your [Profile](#your-account) in the web UI or with `nexctl account api-key generate`. A **read** key is safe to hand to something that only needs to look at your data; a **write** key can change it too. List and revoke keys with `nexctl account api-key list` / `revoke`.

You can point `nexctl` at more than one server using **profiles**: `nexctl profile add`, `nexctl profile list`, and `nexctl profile use <name>` switch between them, and `--profile <name>` targets one for a single command.

Most commands accept a few shared flags: `--json` for machine-readable output (good for piping into other tools), `-q`/`--quiet` for just the bare ids or values, and `-y` to skip confirmation prompts on destructive actions.

### The everyday commands

The groups you'll reach for mirror the web UI:

- **`nexctl game`** — your library: `list` (with filters), `show`, `add`, `edit`, `acquire` (move a wishlist game into your library), `rm`, plus `stats` and `filters` to see what you can filter on.
- **`nexctl pool`** — [play-planning pools](#planning): `list`, `show`, `create`, `edit`, `add`/`remove` games, `queue` and `reorder` to manage the order.
- **`nexctl tag`** — your [tags](#tags): `list`, `create`, `rename`, `rm`.
- **`nexctl sync`** — [storefront sync](#syncing-your-storefront-libraries): `status`, `connect`/`disconnect` a storefront, `run` it, set its `config`, and `review`/`resolve`/`skip` the matches that need your attention.
- **`nexctl doctor`** — [Library health](#library-health): a summary of every check, the games a check flags, and `apply`, `ignore` and `restore`.
- **`nexctl import`** and **`nexctl export`** — the same [import and export](#importing-and-exporting) flows: export your whole library to JSON or CSV, and import from a Nexorious export, another tracker's CSV, or a migration source.

You can also tune your own settings — deal region and notifications — with `nexctl config`. If you run the server, there are operator-only groups too (`admin`, `backup`), covered in the [Admin Guide](admin-guide.md#nexctl-the-api-client). `nexctl mcp` connects an AI chat or agent to your collection; see [Connecting AI chats and agents](#connecting-ai-chats-and-agents).

### Bootstrapping a fresh instance

These commands target the pre-authentication setup and migration zones, so they
do not need an API key. They resolve the server URL from `--url`, then the
current profile's stored URL, then the default (`http://localhost:8000`).

- `nexctl setup admin [--username U] [--password-stdin] [--login]` — create the
  first admin user on a fresh instance. Pending migrations are applied first.
  `--login` also logs in and stores an API key.
- `nexctl setup backups` — list on-disk backups available for restore.
- `nexctl setup restore --file PATH` — upload a backup archive and restore from it.
- `nexctl setup restore <name>` — restore from a named on-disk backup.
- `nexctl migrate` — apply pending migrations on a running server (the web
  migration UI's "Run migrations" button); `nexctl migrate status` shows state.

`nexctl` ships inside the container image, so on a containerized deployment you
can run these via `kubectl exec`/`docker exec` into a fresh instance.

## Connecting AI chats and agents

Nexorious speaks the [Model Context Protocol](https://modelcontextprotocol.io) (MCP), so an AI chat or agent can browse and manage your collection for you: list and look up games, add or edit them, plan pools, manage tags, review sync matches, and find and fix inconsistencies in your library (say, a wishlisted game you already own). The tools it gets mirror the `nexctl` `game`, `pool`, `tag`, `sync`, and `doctor` commands.

Every connection authenticates with an **API key**. Create one on your [Profile](#your-account) or with `nexctl account api-key generate`, and pick its scope deliberately:

- A **read** key lets the agent see your collection but change nothing. The agent is only offered the read tools; if it tries to change something anyway, it's told it needs a write-scoped key.
- A **write** key lets it make changes too: adding and editing games, editing pools and tags, running syncs, and so on.

Start with a read key if you only want answers about your library, and give each agent its own key so you can revoke it on its own.

Your server administrator can turn MCP off. If a client reports that the server has no MCP endpoint, ask them.

### Connecting over HTTP

Most MCP clients can connect straight to the server:

- **URL:** `https://<your-nexorious-host>/api/mcp`
- **Header:** `Authorization: Bearer <your API key>`

For clients configured with an `mcpServers` JSON file, the entry looks like this (the exact shape varies a little between clients):

```json
{
  "mcpServers": {
    "nexorious": {
      "type": "http",
      "url": "https://nexorious.example.com/api/mcp",
      "headers": { "Authorization": "Bearer nxr_your_key_here" }
    }
  }
}
```

**Open WebUI** (an admin sets this up): under **Settings → Admin → Integrations → External Tool Servers**, add a connection with type **MCP (Streamable HTTP)**, the URL above, auth **Bearer**, and your API key in the **Key** field. Every Open WebUI user who enables the tool acts as the owner of that key, so use a key from an account whose collection they should see.

### Connecting a stdio-only client

Some clients can only launch a local program and talk to it over stdin/stdout. For those, `nexctl mcp serve` forwards each message to the server's `/api/mcp` using the API key of your [nexctl profile](#signing-in). `nexctl mcp config` prints ready-made entries for both routes: a stdio one that runs `nexctl mcp serve`, and the HTTP one above, filled in with your server URL and key. That output contains your API key, so keep it private.

`nexctl account login` stores a write key by default. To run an agent read-only, sign in to a separate profile with a read key and generate the config from it:

```bash
nexctl --profile agent account login --scope read
nexctl --profile agent mcp config
```

The stdio entry then runs `nexctl mcp serve --profile agent`, so the agent gets only the read tools while your own profile keeps its write key. `nexctl --profile agent account logout` revokes the agent's key.

### What's not available

- **Connecting a storefront** (`sync connect`), and the **import/export, backup, admin, and settings** commands, aren't offered to agents. Connecting a storefront means handing over passwords or tokens, which shouldn't pass through an AI chat. Do these in the web UI or with `nexctl`.
- **Clients that only support OAuth sign-in**, such as claude.ai custom connectors, can't connect. Nexorious authenticates MCP with API keys only.

## Notifications

Nexorious can let you know when something needs your attention — a sync's credentials have expired, a job finished, and so on. You set this up under **Notifications** on your profile in two parts:

- **Channels** — where notifications go. Each channel is a [Shoutrrr](https://containrrr.dev/shoutrrr/) URL, so any service Shoutrrr supports works here — Telegram, Discord, Slack, Matrix, email, generic webhooks, and [many more](https://containrrr.dev/shoutrrr/v0.8/). Give the channel a name, paste its URL, and add as many as you like.
- **Events** — which kinds of events you want to hear about, grouped by category, each one a simple on/off switch. There's a reset-to-defaults option if you want to start over.

If you don't set up any channels, nothing is sent; the in-app badges (like the sync review count) still work regardless.

### Example: Telegram

A Telegram channel is a single Shoutrrr URL of the form `telegram://<token>@telegram?chats=<chat-id>`. To build one:

1. **Create a bot.** In Telegram, message [@BotFather](https://t.me/BotFather), send `/newbot`, and follow the prompts. It hands you a **bot token** like `123456789:AAExampleTokenString`.
2. **Open a chat with your new bot** (search for the username you gave it).
3. **Generate the URL** with Shoutrrr's helper, which prompts for the token and your chat and prints the finished `telegram://…` string:
   ```bash
   docker run --rm -it containrrr/shoutrrr generate telegram
   ```
4. **Send your bot a message.** This step is easy to miss and the Shoutrrr docs don't spell it out: after running the command, send your bot any message in Telegram. The bot can only discover your chat ID once it has received a message from you, so without it the generator won't find the chat.
5. **Add the channel** on your profile — paste the `telegram://…` URL, give it a name, and save.

The same pattern works for other services: run `docker run --rm -it containrrr/shoutrrr generate <service>`, or check the [Shoutrrr documentation](https://containrrr.dev/shoutrrr/v0.8/) for the URL format.
