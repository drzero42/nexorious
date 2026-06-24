import { Check } from 'lucide-react';

interface ConnectedSummaryProps {
  name?: string;
}

/** The green "Connected as …" box shown when a storefront is connected. */
export function ConnectedSummary({ name }: ConnectedSummaryProps) {
  return (
    <div className="flex items-center gap-3 rounded-lg border bg-muted/50 p-4">
      <Check className="h-5 w-5 text-green-600 dark:text-green-400" />
      <div>
        <p className="font-medium">{name ? `Connected as ${name}` : 'Connected'}</p>
      </div>
    </div>
  );
}
