import type { DateFormatPref } from '@/lib/format-date';
import type { ThemePref } from '@/lib/theme';

export interface Settings {
  dealRegion: string;
  dateFormat: DateFormatPref;
  theme: ThemePref;
}
