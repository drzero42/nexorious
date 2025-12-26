'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { toast } from 'sonner';

export default function ReviewPage() {
  const router = useRouter();

  useEffect(() => {
    toast.info('Review items are now on the Sync page');
    router.replace('/sync');
  }, [router]);

  return (
    <div className="flex min-h-[50vh] items-center justify-center">
      <p className="text-muted-foreground">Redirecting to Sync...</p>
    </div>
  );
}
