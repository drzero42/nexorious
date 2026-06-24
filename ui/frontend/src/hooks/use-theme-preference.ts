import { useTheme } from 'next-themes';
import { useSettings, useUpdateSettings } from './use-settings';
import type { ThemePref } from '@/lib/theme';

/**
 * Reads the user's theme preference (server-backed) and exposes a setter that
 * updates both next-themes (instant) and the server (persisted). Server is the
 * source of truth; ThemeSync reconciles on load.
 */
export function useThemePreference(): { pref: ThemePref; setPref: (t: ThemePref) => void } {
  const { setTheme } = useTheme();
  const { data } = useSettings();
  const update = useUpdateSettings();

  const pref: ThemePref = data?.theme ?? 'system';
  const setPref = (t: ThemePref) => {
    setTheme(t);
    update.mutate({ theme: t });
  };
  return { pref, setPref };
}
