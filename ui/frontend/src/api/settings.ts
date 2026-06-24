import { api } from './client';
import type { Settings } from '@/types/settings';
import type { DateFormatPref } from '@/lib/format-date';
import type { ThemePref } from '@/lib/theme';

interface SettingsApiResponse {
  deal_region: string;
  date_format: DateFormatPref;
  theme: ThemePref;
}

function transform(r: SettingsApiResponse): Settings {
  return { dealRegion: r.deal_region, dateFormat: r.date_format, theme: r.theme };
}

export async function getSettings(): Promise<Settings> {
  return transform(await api.get<SettingsApiResponse>('/settings'));
}

export async function updateSettings(patch: Partial<Settings>): Promise<Settings> {
  const body: Record<string, unknown> = {};
  if (patch.dealRegion !== undefined) body.deal_region = patch.dealRegion;
  if (patch.dateFormat !== undefined) body.date_format = patch.dateFormat;
  if (patch.theme !== undefined) body.theme = patch.theme;
  return transform(await api.patch<SettingsApiResponse>('/settings', body));
}
