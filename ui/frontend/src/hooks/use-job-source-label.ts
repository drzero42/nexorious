import { useAllStorefronts } from '@/hooks';
import { JobSource, NON_STOREFRONT_JOB_SOURCE_LABELS } from '@/types/jobs';

/**
 * Returns a labeller for job sources. Storefront-typed sources (steam,
 * epic-games-store, gog, playstation-store, humble-bundle) resolve to the
 * catalog display_name — the single source of truth. Non-storefront origins
 * (manual, darkadia, csv, ...) use a static label map.
 */
export function useJobSourceLabel(): (source: JobSource | string) => string {
  const { data: storefronts } = useAllStorefronts();
  const byName = new Map((storefronts ?? []).map((s) => [s.name, s.display_name]));
  return (source) =>
    byName.get(source) ?? NON_STOREFRONT_JOB_SOURCE_LABELS[source as JobSource] ?? source;
}
