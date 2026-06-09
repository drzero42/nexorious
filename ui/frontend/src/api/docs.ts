import { apiCall } from './client';

// Docs are served as raw text/markdown, so we read the body as text rather than
// JSON. apiCall handles auth (cookies) and the 401/app-state redirects.
// The path is relative to config.apiUrl (which already includes the "/api"
// prefix), so do NOT prepend "/api" here — that yields "/api/api/docs/...".
export const docsApi = {
  get: (slug: string): Promise<string> =>
    apiCall(`/docs/${encodeURIComponent(slug)}`).then((r) => r.text()),
};
