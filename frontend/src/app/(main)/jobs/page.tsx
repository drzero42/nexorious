'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { toast } from 'sonner';

export default function JobsPage() {
  const router = useRouter();

  useEffect(() => {
    toast.info('Jobs page has been consolidated into Import/Export');
    router.replace('/import-export');
  }, [router]);

  return (
    <div className="flex min-h-[50vh] items-center justify-center">
      <p className="text-muted-foreground">Redirecting to Import/Export...</p>
    </div>
  );
}
