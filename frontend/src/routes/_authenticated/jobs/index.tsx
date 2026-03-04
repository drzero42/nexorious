import { useEffect } from 'react';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { toast } from 'sonner';

export const Route = createFileRoute('/_authenticated/jobs/')({
  component: JobsPage,
});

function JobsPage() {
  const navigate = useNavigate();

  useEffect(() => {
    toast.info('Jobs page has been consolidated into Import/Export');
    navigate({ to: '/import-export', replace: true });
  }, [navigate]);

  return (
    <div className="flex min-h-[50vh] items-center justify-center">
      <p className="text-muted-foreground">Redirecting to Import/Export...</p>
    </div>
  );
}
