import { ExternalLink } from 'lucide-react';
import { Label } from '@/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { editionLabel } from '@/lib/edition-label';
import type { ExternalGameEdition } from '@/types';

interface EditionChoiceProps {
  /** Prefix of the radio items' ids; unique per dialog. */
  name: string;
  editions: ExternalGameEdition[];
  /** The chosen edition's id, or null for all editions. */
  value: string | null;
  onChange: (editionId: string | null) => void;
}

/**
 * EditionChoice lets the user decide all editions of an entry (the default) or
 * one of them alone (#1452). Each edition is listed with its platforms, the
 * storefront's id for it and its store link.
 */
// ALL is the RadioGroup value of "All editions"; edition ids are UUIDs.
const ALL = 'all';

export function EditionChoice({ name, editions, value, onChange }: EditionChoiceProps) {
  return (
    <div className="space-y-2">
      <p id={`${name}-legend`} className="text-sm font-medium">
        Editions
      </p>
      <RadioGroup
        aria-labelledby={`${name}-legend`}
        value={value ?? ALL}
        onValueChange={(v) => onChange(v === ALL ? null : v)}
      >
        <div className="flex items-center gap-2">
          <RadioGroupItem id={`${name}-${ALL}`} value={ALL} />
          <Label htmlFor={`${name}-${ALL}`} className="font-normal">
            All editions
          </Label>
        </div>
        {editions.map((edition) => (
          <div key={edition.id} className="flex items-center gap-2">
            <RadioGroupItem id={`${name}-${edition.id}`} value={edition.id} />
            <Label htmlFor={`${name}-${edition.id}`} className="font-normal">
              Only {editionLabel(edition)}
            </Label>
            {edition.store_url && (
              <a
                href={edition.store_url}
                target="_blank"
                rel="noopener noreferrer"
                aria-label={`Store page of ${editionLabel(edition)}`}
                className="text-muted-foreground hover:text-foreground"
              >
                <ExternalLink className="h-3 w-3" aria-hidden />
              </a>
            )}
          </div>
        ))}
      </RadioGroup>
    </div>
  );
}
