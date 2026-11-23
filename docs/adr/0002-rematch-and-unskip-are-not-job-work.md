# Sync rematch and unskip are not Job work

Rematching an External game used to reset the game's newest Job item to `pending` and queue a worker, even when that item belonged to a sync Job that finished long ago. Unskip created a new one-item sync Job. Both made a user's edit to one game look like a sync: an item changed inside a finished Job, or a second active sync Job appeared for the storefront and announced "sync completed".

We decided that rematch and unskip write the library directly, in the request's own transaction, using the same library write the sync worker uses. They do not create Jobs or items and do not queue item work. The only items a rematch touches are the game's item in Pending review and the unfinished items of its Child games in an active Job: they are completed in the same transaction, because the user's choice settles the whole family.

## Considered options

- **Reopen the finished Job.** Rejected: an old sync becomes the storefront's active sync again and blocks new ones, and its completion runs a second time.
- **A new small Job per action.** Rejected: it bypasses the one-active-sync-per-storefront rule and fires sync notifications for a single edit.

## Consequences

- Only Retry reopens a Job, and only a `completed` one. Retry is refused while another Job with the same duplicate key is active, so it cannot produce the second active sync that ruled out reopening here.
- A rematch is refused (409) while the sync is still working on the game's own item. Its Child games' items never block it: the rematch completes them. (Until sync is on the lifecycle module, #1338 uses a stricter interim rule, because the old code cannot stop a child's worker.)
- Unskipping a game that was never matched only clears the skip. It shows as unmatched, where the user can match it; otherwise the next sync does.
- The storefront view reads "failed" and "needs review" from a game's newest item only, and only while nothing has written the game to the library since that item. Older items are history, except an item in Pending review in an active Job (an older Job that Retry reopened): it keeps that Job from completing, so the game shows for review.
