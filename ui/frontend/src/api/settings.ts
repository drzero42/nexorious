import { api } from './client';
import type { Settings } from '@/types/settings';

interface SettingsApiResponse {
  deal_region: string;
}

function transform(r: SettingsApiResponse): Settings {
  return { dealRegion: r.deal_region };
}

export async function getSettings(): Promise<Settings> {
  return transform(await api.get<SettingsApiResponse>('/settings'));
}

export async function updateSettings(patch: Partial<Settings>): Promise<Settings> {
  const body: Record<string, unknown> = {};
  if (patch.dealRegion !== undefined) body.deal_region = patch.dealRegion;
  return transform(await api.patch<SettingsApiResponse>('/settings', body));
}
