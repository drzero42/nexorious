import { ExternalLink } from 'lucide-react';
import { editionLabel } from '@/lib/edition-label';
import type { ExternalGameEdition } from '@/types';

interface EditionChoiceProps {
  /** Radio group name; unique per dialog. */
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
export function EditionChoice({ name, editions, value, onChange }: EditionChoiceProps) {
  return (
    <fieldset className="space-y-2">
      <legend className="mb-2 text-sm font-medium">Editions</legend>
      <label className="flex items-center gap-2 text-sm">
        <input type="radio" name={name} checked={value === null} onChange={() => onChange(null)} />
        All editions
      </label>
      {editions.map((edition) => (
        <div key={edition.id} className="flex items-center gap-2 text-sm">
          <label className="flex items-center gap-2">
            <input
              type="radio"
              name={name}
              checked={value === edition.id}
              onChange={() => onChange(edition.id)}
            />
            Only {editionLabel(edition)}
          </label>
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
    </fieldset>
  );
}
