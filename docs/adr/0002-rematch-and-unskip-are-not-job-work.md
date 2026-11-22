# Sync rematch and unskip are not Job work

Rematching an External game used to reset the game's newest Job item to `pending` and queue a worker, even when that item belonged to a sync Job that finished long ago. Unskip created a new one-item sync Job. Both made a user's edit to one game look like a sync: an item changed inside a finished Job, or a second active sync Job appeared for the storefront and announced "sync completed".

We decided that rematch and unskip write the library directly, in the request's own transaction, using the same library write the sync worker uses. They do not create Jobs or items and do not queue item work. The only item a rematch touches is one in Pending review for that game (and the items of its waiting children): it is completed in the same transaction, because the user's choice is what that item was waiting for.

## Considered options

- **Reopen the finished Job.** Rejected: an old sync becomes the storefront's active sync again and blocks new ones, and its completion runs a second time.
- **A new small Job per action.** Rejected: it bypasses the one-active-sync-per-storefront rule and fires sync notifications for a single edit.

## Consequences

- Only Retry reopens a Job, and only a `completed` one.
- A rematch is refused (409) while the game has a `pending` item in an active Job; the sync is about to write it.
- Unskipping a game that was never matched does nothing until the next sync matches it.
- The storefront view reads "failed" and "needs review" from a game's newest item only. Older items are history.
