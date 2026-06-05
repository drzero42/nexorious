import { Loader2 } from 'lucide-react';
import { Button } from '@/components/ui/button';

interface ConnectSubmitButtonProps {
  isPending: boolean;
  idleLabel: string;
  pendingLabel: string;
}

/** Full-width form submit button with a spinner while the request is in flight. */
export function ConnectSubmitButton({
  isPending,
  idleLabel,
  pendingLabel,
}: ConnectSubmitButtonProps) {
  return (
    <Button type="submit" disabled={isPending} className="w-full">
      {isPending ? (
        <>
          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          {pendingLabel}
        </>
      ) : (
        idleLabel
      )}
    </Button>
  );
}
