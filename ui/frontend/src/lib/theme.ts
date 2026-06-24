import { Monitor, Sun, Moon, type LucideIcon } from 'lucide-react';

export type ThemePref = 'system' | 'light' | 'dark';

export interface ThemeOption {
  value: ThemePref;
  label: string;
  icon: LucideIcon;
}

export const THEME_OPTIONS: ThemeOption[] = [
  { value: 'system', label: 'System', icon: Monitor },
  { value: 'light', label: 'Light', icon: Sun },
  { value: 'dark', label: 'Dark', icon: Moon },
];
