import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { OwnershipStatus } from '@/types';

const OWNERSHIP_STATUS_OPTIONS: { value: OwnershipStatus; label: string }[] = [
  { value: OwnershipStatus.OWNED, label: 'Owned' },
  { value: OwnershipStatus.BORROWED, label: 'Borrowed' },
  { value: OwnershipStatus.RENTED, label: 'Rented' },
  { value: OwnershipStatus.SUBSCRIPTION, label: 'Subscription' },
  { value: OwnershipStatus.NO_LONGER_OWNED, label: 'No Longer Owned' },
];

/** Editable per-platform ownership/acquired/hours detail. */
export interface PlatformDetail {
  ownershipStatus: OwnershipStatus;
  /** Date in `YYYY-MM-DD` form (empty string when unset). */
  acquiredDate: string;
  hoursPlayed: number;
}

export interface PlatformDetailFieldsProps {
  /** Heading for the card (e.g. "Steam / Windows"). */
  label: string;
  value: PlatformDetail;
  onChange: (next: PlatformDetail) => void;
  /** When true, the hours input is disabled and labelled "(Synced)". */
  hoursSynced?: boolean;
  disabled?: boolean;
}

/**
 * Per-platform ownership/acquired-date/hours editor, shared by the edit page
 * and the add-game wizard so both stay visually and behaviourally in sync.
 */
export function PlatformDetailFields({
  label,
  value,
  onChange,
  hoursSynced = false,
  disabled = false,
}: PlatformDetailFieldsProps) {
  return (
    <div className="p-4 rounded-lg border bg-muted/30">
      <div className="font-medium mb-3">{label}</div>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {/* Ownership Status */}
        <div className="space-y-1">
          <Label className="text-xs text-muted-foreground">Ownership</Label>
          <Select
            value={value.ownershipStatus}
            onValueChange={(v) => onChange({ ...value, ownershipStatus: v as OwnershipStatus })}
            disabled={disabled}
          >
            <SelectTrigger className="h-9">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {OWNERSHIP_STATUS_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {/* Acquired Date */}
        <div className="space-y-1">
          <Label className="text-xs text-muted-foreground">Acquired</Label>
          <Input
            type="date"
            className="h-9"
            value={value.acquiredDate}
            onChange={(e) => onChange({ ...value, acquiredDate: e.target.value })}
            disabled={disabled}
          />
        </div>

        {/* Hours Played */}
        <div className="space-y-1">
          <Label className="text-xs text-muted-foreground">Hours{hoursSynced && ' (Synced)'}</Label>
          <div className="flex items-center gap-2">
            <Input
              type="number"
              min="0"
              step="0.5"
              className="h-9 w-24"
              value={value.hoursPlayed}
              onChange={(e) => onChange({ ...value, hoursPlayed: parseFloat(e.target.value) || 0 })}
              disabled={disabled || hoursSynced}
            />
            <span className="text-sm text-muted-foreground">hrs</span>
          </div>
        </div>
      </div>
    </div>
  );
}
