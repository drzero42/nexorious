'use client';

import { Check, AlertCircle } from 'lucide-react';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Badge } from '@/components/ui/badge';
import type { PlatformMappingSuggestion } from '@/types';
import { cn } from '@/lib/utils';

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
  // Don't render if no items
  if (items.length === 0) {
    return null;
  }

  // Count resolved and unresolved items
  const resolvedCount = items.filter(
    (item) => item.suggestedId || mappings[item.original]
  ).length;
  const unresolvedCount = items.length - resolvedCount;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <h3 className="text-lg font-semibold">{title}</h3>
        <span className="text-sm text-muted-foreground">
          ({items.length} total, {resolvedCount} matched
          {unresolvedCount > 0 && `, ${unresolvedCount} need attention`})
        </span>
      </div>

      <div className="space-y-3">
        {items.map((item) => {
          // Check if this item is resolved (either auto-matched or manually mapped)
          const isAutoMatched = Boolean(item.suggestedId);
          const isManuallyMapped = Boolean(mappings[item.original]);
          const isResolved = isAutoMatched || isManuallyMapped;

          // Get the current value (prefer manual mapping over auto-match)
          const currentValue = mappings[item.original] || item.suggestedId || '';

          return (
            <div
              key={item.original}
              className={cn(
                'flex items-center justify-between gap-4 rounded-lg border p-4',
                isResolved ? 'border-green-200 bg-green-50/50' : 'border-orange-200 bg-orange-50/50'
              )}
            >
              <div className="flex-1">
                <div className="flex items-center gap-2">
                  <span className="font-medium">&quot;{item.original}&quot;</span>
                  {isAutoMatched && !isManuallyMapped && (
                    <Badge variant="secondary" className="bg-green-100 text-green-800">
                      <Check className="mr-1 h-3 w-3" />
                      Auto-matched
                    </Badge>
                  )}
                  {!isResolved && (
                    <Badge variant="secondary" className="bg-orange-100 text-orange-800">
                      <AlertCircle className="mr-1 h-3 w-3" />
                      Needs mapping
                    </Badge>
                  )}
                </div>
                <div className="text-sm text-muted-foreground">
                  {item.count} {item.count === 1 ? 'game' : 'games'}
                </div>
              </div>

              <Select
                value={currentValue}
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
          );
        })}
      </div>
    </div>
  );
}
