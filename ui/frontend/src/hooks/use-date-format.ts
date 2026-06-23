import { useMemo } from 'react';
import { useSettings } from './use-settings';
import {
  formatDate as fmtDate,
  formatDateTime as fmtDateTime,
  type DateFormatPref,
} from '@/lib/format-date';
import { formatRelativeTime as fmtRelative } from '@/types/jobs';

type DateValue = string | number | Date | null | undefined;

/**
 * Returns date formatters bound to the user's date_format preference
 * (from useSettings, defaulting to 'auto'). Use these instead of inline
 * toLocaleDateString/toLocaleString so all dates honour the preference.
 */
export function useDateFormat() {
  const { data } = useSettings();
  const pref: DateFormatPref = data?.dateFormat ?? 'auto';
  return useMemo(
    () => ({
      formatDate: (value: DateValue, nullLabel = '-') => fmtDate(value, pref, nullLabel),
      formatDateTime: (value: DateValue, nullLabel = '-') => fmtDateTime(value, pref, nullLabel),
      formatRelativeTime: (value: string | null, nullLabel = '-') =>
        fmtRelative(value, nullLabel, pref),
    }),
    [pref],
  );
}
