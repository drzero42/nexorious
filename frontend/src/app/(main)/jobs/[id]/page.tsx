'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { toast } from 'sonner';

export default function JobDetailPage() {
  const router = useRouter();

  useEffect(() => {
    toast.info('Job details are now shown inline on Import/Export');
    router.replace('/import-export');
  }, [router]);

  return (
    <div className="flex min-h-[50vh] items-center justify-center">
      <p className="text-muted-foreground">Redirecting to Import/Export...</p>
    </div>
  );
}
