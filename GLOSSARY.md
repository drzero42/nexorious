# Nexorious

A self-hosted tracker for a personal video-game collection: the games a user owns, wants, and plays, kept in step with their storefront libraries.

## Background work

**Job**:
One run of background work started by a user or the system (a sync, an import, an export, or a maintenance refresh), tracked from creation until it has Ended.
_Avoid_: task, run, River job (the queue's unit of execution, of which one Job may have many)

**Job item**:
One unit of work inside a Job: one external game in a sync, one game in an import, one game or one storefront in a refresh.
_Avoid_: task, row, entry

**Job kind**:
The sort of a Job (sync, import from a given source, export, metadata refresh, store-link refresh), which decides how its items are worked and what happens when it ends.
_Avoid_: job type (one input to the kind, not the whole of it)

**Dispatch**:
The first phase of a Job, in which its items are created. A Job cannot complete until its dispatch is done, even if every item created so far has Ended. An export has no items: its dispatch is all of its work.

**Item status**:
While a Job item is Open, its status says who acts next: a worker (`pending`) or the user (`pending_review`). It never records a decision: a Resolution is kept apart from it.

**Pending review**:
A Job item a worker could not match to an IGDB game with confidence, waiting for the user to Resolve or Skip it. It keeps its Job from completing.
_Avoid_: unmatched, needs review

**Resolution**:
The IGDB game the user chose for an item in Pending review. Resolving lets the item end. In a sync the choice is made by a Rematch.

**Skip**:
The user's decision not to add a game to the library. It ends the game's Open item; in a sync it also stands for later syncs, until the user removes it.

**Ended**:
A Job or Job item whose work is over: completed, failed, skipped (items only), or cancelled. Nothing acts on it again unless the user asks for a Retry. A Job item that has not ended is **Open**.
_Avoid_: finished (a game's play status), terminal, done, unfinished

**Completion**:
The moment a Job becomes completed: its dispatch is done and none of its items wait on a worker or on the user.

**Cancel**:
A user stopping a Job before completion. Its Open items are cancelled with it.

**Fail**:
A Job ending because of an error. Its Open items are cancelled, as with Cancel.

**Ending effects**:
What follows from a Job being completed or failed, or coming to wait on the user: its notifications, a sync's diff, an import's pools. They happen because the Job reached that point, never as part of an item's work.

**Retry**:
Handing a Job's failed items back to a worker. Retrying reopens a completed Job until its next Completion. A failed or cancelled Job cannot be retried.

## Storefront sync

**External game**:
A game as it appears in a connected storefront's library, matched to at most one IGDB game.
_Avoid_: storefront game

**Child game**:
An External game that is another edition of an External game already known for the same storefront, its parent. It follows the parent's match instead of being matched itself.
_Avoid_: sibling

**Unmatched**:
An External game that is not skipped and points to no IGDB game, and has no item in Pending review: for instance one unskipped before it was ever matched. It waits for the user to Rematch it, or for the next sync to match it while it is still in the storefront library.
_Avoid_: pending review (an item in a Job waiting for the user; an Unmatched game is waiting for nothing)

**Rematch**:
Choosing or changing which IGDB game an External game points to. The library is updated at once; a Rematch is not a Job.
_Avoid_: resolve (a Resolution is the choice; in a sync, the Rematch is the act that makes it)

## Library health

**Library health**:
The set of Checks run over a user's collection to find data that looks wrong or out of date.
_Avoid_: library smells (a Smell is one tier of Checks, not the whole feature), doctor (the name of the `nexctl` command, not of the feature)

**Check**:
One rule in Library health, such as "Wishlisted yet owned". It flags the games that break it, and belongs to one of two tiers: Inconsistency or Smell.
_Avoid_: detector, smell (a tier of Checks, not one Check)

**Flagged game**:
A game that a Check currently reports. It stays flagged until its data no longer breaks the Check, or until it is Ignored for that Check.
_Avoid_: finding, issue, flagged item

**Inconsistency**:
The tier of Checks whose Flagged games have data that is wrong and probably needs fixing.

**Smell**:
The tier of Checks whose Flagged games have data that looks out of date or incomplete and might be worth updating, but is not wrong.
_Avoid_: suggestion (a pool's matching game), nudge

**Apply**:
Running an auto-fixable Check's fix on some or all of its Flagged games. It is always scoped to one Check. Checks that are not auto-fixable leave the fix to the user.
_Avoid_: fix all, apply suggestion (a Suggestion belongs to a pool)

**Ignore**:
Telling one Check to stop flagging one game. Other Checks still apply to the game. **Restore** undoes it.
_Avoid_: dismiss, un-dismiss

## Collection

**IGDB game**:
A game as recorded in IGDB, the shared catalogue Nexorious takes titles, covers and other details from. Many users' Games can point to the same IGDB game.
_Avoid_: game (the user's own record), title, metadata

**Game**:
A user's own record of one IGDB game: their play status, rating, notes, tags and Copies. A user has at most one Game per IGDB game.
_Avoid_: user game, entry, title

**Collection**:
All of a user's Games: the Library and the Wishlist together.
_Avoid_: account, catalogue

**Library**:
The Games in a user's Collection that are not on the Wishlist.
_Avoid_: owned games (a Library Game's Copies may be borrowed, rented or no longer owned)

**Wishlist**:
The Games a user wants but has not Acquired. A Game leaves the Wishlist when it is Acquired.

**Copy**:
One way a user has a Game: one Platform, optionally the Storefront it came from, with its own Ownership status, acquired date and hours played. A Game can have several Copies, for instance on PC from Steam and on PS5 as a physical disc.
_Avoid_: platform (the hardware a Copy runs on), platform row, platform entry, ownership

**Platform**:
What a Copy runs on, such as PC (Windows) or PlayStation 5.
_Avoid_: storefront (where a Copy came from), system, console

**Storefront**:
Where a Copy came from, such as Steam, GOG, the PlayStation Store, or Physical for a disc or cartridge. A Copy may have none recorded.
_Avoid_: platform (what a Copy runs on), store, shop, launcher

**Ownership status**:
How the user has a Copy: owned, borrowed, rented, through a subscription, or no longer owned.

**Acquire**:
Recording that the user got a Game, by adding one or more Copies, whether by hand, through a sync or through an import. An Acquired Game leaves the Wishlist and joins the Library.
_Avoid_: move to library, promote (a Candidate entering a Queue), buy

**Remove**:
Taking a Game out of the Collection, with its Copies. The IGDB game is untouched.
_Avoid_: delete (for things that cease to exist, such as a tag or a pool)

**Loved**:
A user's mark that a Game is a favourite.
_Avoid_: favorite, favourite, starred

**Tag**:
A label of the user's own choosing, attached to Games. Deleting a Tag leaves the Games alone.
_Avoid_: label, category

## Play status

**Play status**:
Where the user is with a Game: Not Started, In Progress, Completed, Mastered, Dominated, Shelved, Dropped, or Replay. A Game has one play status, whatever its Copies.
_Avoid_: progress, state

**Finished**:
A game whose play status means the user has no further plans to play it: Completed, Mastered, Dominated, or Dropped. A Finished game leaves every pool.
_Avoid_: ended (a Job's state), terminal, done

**Beaten**:
A Finished game the user played through: Completed, Mastered, or Dominated, but not Dropped.
_Avoid_: finished (also includes Dropped), completed (one of the three statuses)

## Planning

**Pool**:
A named collection of games the user plans to play, in their library or wishlist. A game can be in several pools at once.
_Avoid_: list, backlog

**Queue**:
The ordered part of a pool: the games the user has placed in the order they mean to play them. Its first game is **Play Next**.
_Avoid_: up next

**Candidate**:
A game in a pool that is not in its Queue yet. Promoting it places it in the Queue; demoting it takes it back out.
_Avoid_: candidate for any other meaning (an ambiguous name's possible matches, IGDB games offered in match review)

**Suggestion**:
A game from the user's library or wishlist that matches a pool's filter but is not in the pool yet. Finished games are never Suggestions.
_Avoid_: match, recommendation
