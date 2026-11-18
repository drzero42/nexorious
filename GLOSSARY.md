# Nexorious

A self-hosted tracker for a personal video-game collection: the games a user owns, wants, and plays, kept in step with their storefront libraries.

## Background work

**Job**:
One run of background work started by a user or the system (a sync, an import, an export, or a maintenance refresh), tracked from creation until it is finished.
_Avoid_: task, run, River job (the queue's unit of execution, of which one Job may have many)

**Job item**:
One unit of work inside a Job: one external game in a sync, one game in an import, one game in a refresh.
_Avoid_: task, row, entry

**Job kind**:
The sort of a Job (sync, import from a given source, export, metadata refresh, store-link refresh), which decides how its items are worked, whether it can be retried, and what happens when it finishes.
_Avoid_: job type (one input to the kind, not the whole of it)

**Dispatch**:
Creating all of a Job's items. A Job cannot complete until its dispatch is done, even if every item created so far is finished.

**Item status**:
While a Job item is unfinished, its status says who acts next: a worker (`pending`, `processing`) or the user (`pending_review`). It never records a decision; a Resolution is kept apart from it, so a resolved item is `pending` again until a worker finishes it.

**Pending review**:
A Job item a worker could not match to an IGDB game with confidence, waiting for the user to Resolve or Skip it. It keeps its Job from completing.
_Avoid_: unmatched, needs review

**Resolution**:
The IGDB game the user chose for an item in Pending review. Resolving hands the item back to a worker to finish.

**Skip**:
The user's decision to drop an item in Pending review without adding it to the library.

**Finished**:
A Job or Job item that nobody will act on again: completed, failed, skipped (items only), or cancelled.
_Avoid_: terminal, done

**Completion**:
The moment a Job becomes completed: its dispatch is done and none of its items wait on a worker or on the user.

**Cancel**:
A user stopping a Job before completion. Its unfinished items are cancelled with it.

**Fail**:
A Job ending because of an error. Its unfinished items are cancelled, as with Cancel.

**Retry**:
Handing a Job's failed items back to a worker. Retrying reopens a finished Job until its next Completion. Not every Job kind can be retried.

## Storefront sync

**External game**:
A game as it appears in a connected storefront's library, matched to at most one IGDB game.
_Avoid_: storefront game

**Rematch**:
Changing which IGDB game an already matched External game points to.
_Avoid_: resolve (that is for items in Pending review)
