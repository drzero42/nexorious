import type { ExternalGameEdition } from '@/types';

/**
 * editionLabel tells an edition (or a sync entry) apart from same-title ones:
 * its platforms and the storefront's id for it (#1452).
 */
export function editionLabel(edition: Pick<ExternalGameEdition, 'platforms' | 'external_id'>) {
  const platforms = edition.platforms.length > 0 ? edition.platforms.join(', ') : 'No platforms';
  return `${platforms} · ID ${edition.external_id}`;
}
