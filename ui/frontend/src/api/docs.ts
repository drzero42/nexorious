import { apiCall } from './client';

// Docs are served as raw text/markdown, so we read the body as text rather than
// JSON. apiCall handles auth (cookies) and the 401/app-state redirects.
export const docsApi = {
  get: (slug: string): Promise<string> =>
    apiCall(`/api/docs/${encodeURIComponent(slug)}`).then((r) => r.text()),
};
