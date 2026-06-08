export interface DealLinks {
  /** IsThereAnyDeal search (PC). Region follows the user's ITAD account. */
  itad: string;
  /** PSprices search (console), scoped to the user's deal region. */
  psprices: string;
}

/**
 * Build deal-site deep links for a game title. Both are plain title searches
 * (no live pricing); the user picks the ecosystem at click time. The psprices
 * link is region-scoped via `dealRegion` (psprices region code, e.g. "us").
 */
export function buildDealLinks(title: string, dealRegion = 'us'): DealLinks {
  const q = encodeURIComponent(title);
  return {
    itad: `https://isthereanydeal.com/search/?q=${q}`,
    psprices: `https://psprices.com/region-${dealRegion}/games/?q=${q}`,
  };
}
