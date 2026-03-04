import { useEffect } from 'react';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { toast } from 'sonner';

export const Route = createFileRoute('/_authenticated/review')({
  component: ReviewPage,
});

function ReviewPage() {
  const navigate = useNavigate();

  useEffect(() => {
    toast.info('Review items are now on the Sync page');
    navigate({ to: '/sync', replace: true });
  }, [navigate]);

  return (
    <div className="flex min-h-[50vh] items-center justify-center">
      <p className="text-muted-foreground">Redirecting to Sync...</p>
    </div>
  );
}
