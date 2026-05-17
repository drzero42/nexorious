# Needs Review: Alphabetical Sort + Pagination Fix

## Problem

Two independent bugs affecting the "Needs Review" section under Item Details on sync pages:

1. **Items are not sorted alphabetically.** The backend orders all `job_items` by `created_at ASC` regardless of status, so review items appear in arrival order — unhelpful when scanning a long list manually.
2. **Pagination is broken end-to-end.** The frontend sends `page_size=N` but the backend reads `per_page`; the backend returns `total_pages` but the frontend type declares `pages`. The result: page size is silently ignored (always defaults to 20), and `data.pages` is always `undefined`, so the Next/Previous buttons never render.

## Design

### Backend — conditional sort (`internal/api/jobs.go`)

`HandleGetJobItems` currently applies `ORDER BY created_at ASC` unconditionally. Change it to order by `source_title ASC` when the `status` query param is `pending_review`. All other statuses keep `created_at ASC`.

Because items are paginated server-side, sorting at the DB level is the only way to guarantee global alphabetical order across pages.

Re-sorting on new arrivals is already handled: `StatusSection` refetches whenever `count` changes (existing `useEffect`), so new items flow into the sorted list automatically.

### Frontend — fix API field name mismatches (`ui/frontend/src/api/jobs.ts`)

Three changes in the API client:

- `JobItemListApiResponse`: rename `pages → total_pages` and `page_size → per_page` to match the actual JSON keys the backend emits.
- `getJobItems` query params: send `per_page` instead of `page_size`.
- `getJobItems` response mapping: use `response.total_pages` for the `pages` field of the public `JobItemListResponse`.

No changes to `JobItemListResponse` (the public frontend type) or any component — the pagination UI in `StatusSection` is already correct once real data arrives.

## Scope

- No new API surface, no DB migrations, no schema changes.
- No changes to other status sections (they keep `created_at ASC` ordering).
- No frontend component changes beyond the API client fix.
