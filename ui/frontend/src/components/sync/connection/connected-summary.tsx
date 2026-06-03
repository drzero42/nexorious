import { Check } from 'lucide-react';

interface ConnectedSummaryProps {
  name?: string;
  secondary?: string;
  /** Render the secondary line in a monospace font (used for opaque IDs). */
  secondaryMono?: boolean;
}

/** The green "Connected as …" box shown when a storefront is connected. */
export function ConnectedSummary({
  name,
  secondary,
  secondaryMono = false,
}: ConnectedSummaryProps) {
  return (
    <div className="flex items-center gap-3 rounded-lg border bg-muted/50 p-4">
      <Check className="h-5 w-5 text-green-600" />
      <div>
        <p className="font-medium">Connected as {name}</p>
        {secondary && (
          <p className={`text-sm text-muted-foreground${secondaryMono ? ' font-mono' : ''}`}>
            {secondary}
          </p>
        )}
      </div>
    </div>
  );
}
