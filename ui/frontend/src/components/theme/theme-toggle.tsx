import { Sun, Moon, Monitor } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useThemePreference } from '@/hooks';
import { THEME_OPTIONS, type ThemePref } from '@/lib/theme';

const ICONS: Record<ThemePref, typeof Sun> = {
  system: Monitor,
  light: Sun,
  dark: Moon,
};

export function ThemeToggle() {
  const { pref, setPref } = useThemePreference();
  const CurrentIcon = ICONS[pref];

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label="Change theme">
          <CurrentIcon className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-40">
        {THEME_OPTIONS.map((o) => {
          const Icon = o.icon;
          return (
            <DropdownMenuItem
              key={o.value}
              onClick={() => setPref(o.value)}
              className="cursor-pointer"
            >
              <Icon className="mr-2 h-4 w-4" />
              <span>{o.label}</span>
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
