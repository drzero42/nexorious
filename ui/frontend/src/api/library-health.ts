import { api } from './client';

// The shared `api` client prepends config.apiUrl ('/api'), so paths here are
// relative to that — do NOT include a leading '/api' or it doubles to '/api/api'.
const BASE = '/library/smells';
const MAX_IDS = 200;

export type SmellTier = 'inconsistency' | 'nudge';

export interface SmellSummaryItem {
  id: string;
  title: string;
  description: string;
  tier: SmellTier;
  auto_fixable: boolean;
  count: number;
}

export interface FlaggedItem {
  user_game_id: string;
  game_id: number;
  title: string;
  cover_art_url?: string;
  platform_row_id?: string;
  platform?: string;
  storefront?: string;
  suggested_storefront?: string;
  suggested_status?: string;
  detail?: string;
}

export interface FlaggedListResponse {
  items: FlaggedItem[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

export interface IgnoredItem {
  user_game_id: string;
  title: string;
  created_at: string;
}

export interface IgnoredListResponse {
  items: IgnoredItem[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

export interface ApplyResult {
  applied: number;
  skipped: number;
}

export function getSmellSummary(): Promise<SmellSummaryItem[]> {
  return api.get<SmellSummaryItem[]>(BASE);
}

export function getSmellItems(
  checkID: string,
  perPage = MAX_IDS,
  page = 1,
): Promise<FlaggedListResponse> {
  return api.get<FlaggedListResponse>(`${BASE}/${checkID}`, {
    params: { page, per_page: perPage },
  });
}

export function getIgnoredItems(
  checkID: string,
  perPage = MAX_IDS,
  page = 1,
): Promise<IgnoredListResponse> {
  return api.get<IgnoredListResponse>(`${BASE}/${checkID}/ignored`, {
    params: { page, per_page: perPage },
  });
}

// Applies in chunks of <=200 (the API cap) and aggregates the result.
export async function applySmell(checkID: string, userGameIds: string[]): Promise<ApplyResult> {
  let applied = 0;
  let skipped = 0;
  for (let i = 0; i < userGameIds.length; i += MAX_IDS) {
    const chunk = userGameIds.slice(i, i + MAX_IDS);
    const res = await api.post<ApplyResult>(`${BASE}/${checkID}/apply`, {
      user_game_ids: chunk,
    });
    applied += res.applied;
    skipped += res.skipped;
  }
  return { applied, skipped };
}

// Walks every page (per_page=200) and returns all flagged user_game_ids.
export async function fetchAllFlaggedIds(checkID: string): Promise<string[]> {
  const ids: string[] = [];
  let page = 1;
  for (;;) {
    const res = await getSmellItems(checkID, MAX_IDS, page);
    ids.push(...res.items.map((it) => it.user_game_id));
    if (res.items.length === 0 || page >= res.pages) break;
    page += 1;
  }
  return ids;
}

export async function applyAllSmell(checkID: string): Promise<ApplyResult> {
  const ids = await fetchAllFlaggedIds(checkID);
  if (ids.length === 0) return { applied: 0, skipped: 0 };
  return applySmell(checkID, ids);
}

export function ignoreSmell(checkID: string, userGameIds: string[]): Promise<{ ignored: number }> {
  return api.post<{ ignored: number }>(`${BASE}/${checkID}/ignore`, {
    user_game_ids: userGameIds,
  });
}

export function restoreSmell(
  checkID: string,
  userGameIds: string[],
): Promise<{ restored: number }> {
  return api.delete<{ restored: number }>(`${BASE}/${checkID}/ignore`, {
    body: JSON.stringify({ user_game_ids: userGameIds }),
  });
}
