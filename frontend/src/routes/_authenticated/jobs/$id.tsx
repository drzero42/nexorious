import { useEffect } from 'react';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { toast } from 'sonner';

export const Route = createFileRoute('/_authenticated/jobs/$id')({
  component: JobDetailPage,
});

function JobDetailPage() {
  const navigate = useNavigate();

  useEffect(() => {
    toast.info('Job details are now shown inline on Import/Export');
    navigate({ to: '/import-export', replace: true });
  }, [navigate]);

  return (
    <div className="flex min-h-[50vh] items-center justify-center">
      <p className="text-muted-foreground">Redirecting to Import/Export...</p>
    </div>
  );
}
