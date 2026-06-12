import { Input } from '@/components/ui/input';

/** Predefined color palette shared by tags and pools. */
export const COLOR_PALETTE = [
  '#EF4444',
  '#F97316',
  '#F59E0B',
  '#EAB308',
  '#84CC16',
  '#22C55E',
  '#10B981',
  '#14B8A6',
  '#06B6D4',
  '#0EA5E9',
  '#3B82F6',
  '#6366F1',
  '#8B5CF6',
  '#A855F7',
  '#D946EF',
  '#EC4899',
  '#F43F5E',
  '#6B7280',
];

export function ColorPicker({
  value,
  onChange,
}: {
  value: string;
  onChange: (color: string) => void;
}) {
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <div className="h-8 w-8 rounded-md border" style={{ backgroundColor: value }} />
        <Input
          type="text"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="w-28 font-mono text-sm"
          placeholder="#000000"
        />
      </div>
      <div className="grid grid-cols-9 gap-1">
        {COLOR_PALETTE.map((color) => (
          <button
            key={color}
            type="button"
            className={`h-6 w-6 rounded-md border-2 transition-transform hover:scale-110 ${
              value === color ? 'border-foreground' : 'border-transparent'
            }`}
            style={{ backgroundColor: color }}
            onClick={() => onChange(color)}
            aria-label={`Select color ${color}`}
          />
        ))}
      </div>
    </div>
  );
}
