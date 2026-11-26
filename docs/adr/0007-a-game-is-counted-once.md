# A game is counted once, whatever platforms and editions the user has

One game can be stored as several rows. In the library it has one row per platform and storefront it is owned on. In a storefront's library it has one External game per edition the storefront sells separately: PSN gives the PS4 and PS5 versions of a game different ids, so they arrive as two External games, editions of one game. Counts that were labelled "games" had come to count different things: the library total counted games, while the sync status counted editions (#1438).

We decided that "game" always means the game itself. Anything that shows the user a number of games, or a list of games, counts and lists each game once: a game owned on PS4 and PS5 is one game. Platforms, storefronts and editions are details of a game, never more games.

- In the library, a game is one library entry, whatever platforms it has.
- In a storefront's sync views, a game is one set of editions ([ADR 0008](0008-derived-editions.md)). Its editions are not counted or listed on their own.
- A record of what a sync wrote to the library ("Added to library", "Already in library", "Status changed") counts library entries, also when it is shown in a sync view. What a sync records about the storefront ("Removed from storefront", "Skipped") counts sets of editions.

## Considered options

- **Count what is stored.** Rejected: the number would depend on how each storefront numbers its releases. The same game on two PlayStation consoles would count as two games on PSN, and as one on a storefront that sells it under one id.
- **Count a game once per platform.** Rejected for the same reason: it measures how the user owns their games, not how many they have.

## Consequences

- A number that counts something else must not be called games. A Job's progress counts Job items, and a sync has one item per edition, so it reports items.
- Breakdowns do not add up to the total. A game on two platforms is counted under each platform, and a game owned on two storefronts is counted in each storefront's sync view, but once in the library total.
- Within one breakdown value a game still counts once: a game owned on PC through two storefronts is one PC game.
- A storefront's sync view and the library can disagree. Editions are grouped by their current title, match and skip, not by how they were first seen: two editions whose titles come to differ, or that come to differ on match or skip, become two entries. Two External games with different titles that the user matched to the same IGDB game are therefore two games in the sync view and one game in the library. Likewise, an edition the user skipped alone (#1452) makes one game show as two entries, one of them in Skipped, while the library has it once.
- That some games are owned on several platforms stays visible: it is shown on the game, not by counting it more than once.
