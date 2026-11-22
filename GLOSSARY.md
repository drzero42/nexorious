# Nexorious

A self-hosted tracker for a personal video-game collection: the games a user owns, wants, and plays, kept in step with their storefront libraries.

## Background work

**Job**:
One run of background work started by a user or the system (a sync, an import, an export, or a maintenance refresh), tracked from creation until it is finished.
_Avoid_: task, run, River job (the queue's unit of execution, of which one Job may have many)

**Job item**:
One unit of work inside a Job: one external game in a sync, one game in an import, one game or one storefront in a refresh.
_Avoid_: task, row, entry

**Job kind**:
The sort of a Job (sync, import from a given source, export, metadata refresh, store-link refresh), which decides how its items are worked and what happens when it finishes.
_Avoid_: job type (one input to the kind, not the whole of it)

**Dispatch**:
The first phase of a Job, in which its items are created. A Job cannot complete until its dispatch is done, even if every item created so far is finished. An export has no items: its dispatch is all of its work.

**Item status**:
While a Job item is unfinished, its status says who acts next: a worker (`pending`) or the user (`pending_review`). It never records a decision: a Resolution is kept apart from it.

**Pending review**:
A Job item a worker could not match to an IGDB game with confidence, waiting for the user to Resolve or Skip it. It keeps its Job from completing.
_Avoid_: unmatched, needs review

**Resolution**:
The IGDB game the user chose for an item in Pending review. Resolving lets the item be finished. In a sync the choice is made by a Rematch.

**Skip**:
The user's decision not to add a game to the library. It finishes the item that was waiting for a decision; in a sync it also stands for later syncs, until the user removes it.

**Finished**:
A Job or Job item whose work has ended: completed, failed, skipped (items only), or cancelled. Nothing acts on it again unless the user asks for a Retry.
_Avoid_: terminal, done

**Completion**:
The moment a Job becomes completed: its dispatch is done and none of its items wait on a worker or on the user.

**Cancel**:
A user stopping a Job before completion. Its unfinished items are cancelled with it.

**Fail**:
A Job ending because of an error. Its unfinished items are cancelled, as with Cancel.

**Finishing effects**:
What follows from a Job being completed or failed: its notifications, a sync's diff, an import's pools. They happen because the Job finished, never as part of an item's work.

**Retry**:
Handing a Job's failed items back to a worker. Retrying reopens a completed Job until its next Completion. A failed or cancelled Job cannot be retried.

## Storefront sync

**External game**:
A game as it appears in a connected storefront's library, matched to at most one IGDB game.
_Avoid_: storefront game

**Child game**:
An External game that is another edition of an External game already known for the same storefront, its parent. It follows the parent's match instead of being matched itself.
_Avoid_: sibling

**Rematch**:
Choosing or changing which IGDB game an External game points to. The library is updated at once; a Rematch is not a Job.
_Avoid_: resolve (a Resolution is the choice; in a sync, the Rematch is the act that makes it)
