# A game's editions are derived, not stored

Some storefronts sell the platform releases of one game under separate ids: PSN gives the PS4 and PS5 versions different title ids, so they arrive as two External games. Until #1450 the second one became a Child game of the first (`external_games.parent_id`). The child carried a copy of its parent's match and skip, and a set of rules kept the copies equal and kept every family attached to a parent still in the storefront library: adoption with an eligibility rule, a skip repair at every dispatch, promotion of a child when its unmatched parent left, a refusal of Rematch, Skip and Unskip on a child, a parent-first lock order, and a child's item waiting on its parent's item. Those rules were where the sync bugs of the months before came from (#1339, #1419, #1425, #1437).

We decided to derive which External games belong together instead of storing it (#1434, #1449):

> The **editions** of an External game are the External games of the same user and storefront that have the same title, the same match and the same skip.

Every External game has its own match and its own skip. Two games are editions for as long as they agree. The key is defined once, in `internal/externalgame`, and every query that needs it is built from that definition. Four rules make editions behave as one game:

1. **Precedent.** When a sync works on a game without a decision, and a same-title game has one, the game takes the match and the skip of the oldest such game (`created_at`, then id) and is not searched. The copy is made once.
2. **Fan-out.** Rematch, Skip and Unskip act on every edition of the game they are given, through any edition's id, and lock the editions in id order. Asked to (`only_this_edition=true`, #1452), they act on the given edition alone, still locking the whole set.
3. **One row.** The storefront list and the review badge show a set of editions as one entry. It needs review, has failed or is hidden as in flight when any edition does.
4. **Settle.** When a search matches a game with confidence, its same-title games of that sync waiting in Pending review without a decision take that match and their items are completed. This is best effort.

## Considered options

- **Keep the stored tree.** Rejected: every combination of a child that differs from its parent, or whose parent has left, is an invalid state that needs code to prevent, detect or repair it, and each of those rules has had bugs.
- **Keep the tree but drop the copies** (a child reads its parent's match). Rejected: the parent link, adoption, promotion and the lock order stay, and a child still cannot be decided on its own.
- **A stored flat group** (#1426). Rejected: a group keeps one match per title, and one production library has two same-title games on one storefront that are two different IGDB games. Taking two same-title games apart would need groups with their own identity, plus move and merge operations.
- **Derived editions.** Chosen: there are no invalid states. Two same-title games agree and are one entry, or differ and are two. Deciding one edition on its own makes it differ (#1452), and rematching it to the other's game makes them agree again, with no new state.

## Consequences

- The key is the exact current title within one user and one storefront. A same-title game on another storefront decides nothing, and editions whose titles come to differ become separate entries.
- Precedent applies whenever a game has no decision, not only when it is new, so two same-title games stay apart only while both have a decision. A game the user has matched or skipped is never changed by precedent.
- Every edition keeps its own Job item. Two undecided editions are two items in Pending review and one entry, and one Rematch completes both. Rematch is refused (409) only while the given game's own item is pending ([ADR 0002](0002-rematch-and-unskip-are-not-job-work.md)).
- A new edition of a skipped game now gets a Job item, which takes the skip in the match stage and ends skipped.
- Locking a set is optimistic: the set is locked by the game's state as read, then read again, and locked again when it changed (three tries, then 409). Two ways into or out of a set are not closed and are accepted: the dispatch rewrites a title at every sync without this lock, and another Rematch can move a same-title game onto the same match.
- Settle runs after the matched edition's transition has committed. If it fails, the other editions stay in review for the user, and a needs-review notification can go out for an item that settle then completes.
- No item waits on another item any more, so the waiting rules in `internal/joblifecycle` have no user (#1451).
- `external_games.parent_id` is dropped. The down migration brings the column back empty, so a downgrade loses the families.
- Taking a set apart is a difference in match or skip and nothing more (#1452). A Rematch or Skip of one edition alone makes it differ, so it is its own entry; deciding it back to what the others have makes it one of them again. There is no split or merge operation and nothing is stored about either. A game is kept apart only while it has a decision: an undecided edition beside one decided alone takes the oldest same-title decision when the sync next works on it, which may be the one just made. Keeping an undecided game apart would need a stored flag, and would be a new decision.
- Two editions on one platform and storefront share one library link, held by one of them. Before an edition leaves its set by a Rematch or Skip alone, the editions that stay are written to the library again, so one of them takes the shared link over and its library game is not orphaned. The link keeps the larger of the two editions' hours: playtime on a shared link is not taken apart.
- The ignored list (`GET /api/sync/ignored`) and the sync status count show a set as one entry too (#1438), and a sync's change lists list a game once (#1443).
