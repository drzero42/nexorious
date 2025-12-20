'use client';

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import type { PlatformMappingSuggestion } from '@/types';

interface MappingSectionProps {
  title: string;
  items: PlatformMappingSuggestion[];
  options: { id: string; display_name: string }[];
  mappings: Record<string, string>;
  onMappingChange: (original: string, resolvedId: string) => void;
}

export function MappingSection({
  title,
  items,
  options,
  mappings,
  onMappingChange,
}: MappingSectionProps) {
  // Filter to only show unresolved items (no suggestedId)
  const unresolvedItems = items.filter((item) => !item.suggestedId);

  // Don't render if no unresolved items
  if (unresolvedItems.length === 0) {
    return null;
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <h3 className="text-lg font-semibold">{title}</h3>
        <span className="text-sm text-muted-foreground">
          ({unresolvedItems.length} unresolved)
        </span>
      </div>

      <div className="space-y-3">
        {unresolvedItems.map((item) => (
          <div
            key={item.original}
            className="flex items-center justify-between gap-4 rounded-lg border p-4"
          >
            <div className="flex-1">
              <div className="font-medium">&quot;{item.original}&quot;</div>
              <div className="text-sm text-muted-foreground">
                {item.count} {item.count === 1 ? 'game' : 'games'}
              </div>
            </div>

            <Select
              value={mappings[item.original] || ''}
              onValueChange={(value) => onMappingChange(item.original, value)}
            >
              <SelectTrigger className="w-[250px]">
                <SelectValue placeholder={`Select ${title.toLowerCase().slice(0, -1)}`} />
              </SelectTrigger>
              <SelectContent>
                {options.map((option) => (
                  <SelectItem key={option.id} value={option.id}>
                    {option.display_name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        ))}
      </div>
    </div>
  );
}
