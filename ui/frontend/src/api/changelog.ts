import { apiCall } from './client';

export interface ChangelogGroup {
  title: string;
  items: string[];
}

export interface ChangelogEntry {
  version: string;
  date: string;
  groups: ChangelogGroup[];
}

export interface ChangelogResult {
  available: boolean;
  current: string;
  last_seen?: string;
  markdown: string;
  entries: ChangelogEntry[];
}

// Paths are relative to config.apiUrl (which already includes "/api"); do NOT
// prepend "/api" here.
export const changelogApi = {
  unseen: (): Promise<{ has_unseen: boolean }> =>
    apiCall('/changelog/unseen').then((r) => r.json()),
  get: (params?: { all?: boolean; since?: string }): Promise<ChangelogResult> => {
    const qs = new URLSearchParams();
    if (params?.since) qs.set('since', params.since);
    else if (params?.all) qs.set('range', 'all');
    const suffix = qs.toString() ? `?${qs.toString()}` : '';
    return apiCall(`/changelog${suffix}`).then((r) => r.json());
  },
};
