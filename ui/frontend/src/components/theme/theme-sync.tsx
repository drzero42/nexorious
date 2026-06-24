import { useEffect } from 'react';
import { useTheme } from 'next-themes';
import { useSettings } from '@/hooks';

/**
 * One-way bridge: pushes the server-stored theme preference into next-themes
 * once settings load (and whenever it changes), so the server is the source of
 * truth. Renders nothing. Mounted inside the authenticated layout so the
 * authenticated /api/settings endpoint is never hit for anonymous visitors.
 */
export function ThemeSync() {
  const { data } = useSettings();
  const { theme, setTheme } = useTheme();

  useEffect(() => {
    if (data?.theme && data.theme !== theme) {
      setTheme(data.theme);
    }
  }, [data?.theme, theme, setTheme]);

  return null;
}
