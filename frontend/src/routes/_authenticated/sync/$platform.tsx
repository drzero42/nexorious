import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/sync/$platform')({
  component: () => <div>Sync Platform (migrating...)</div>,
});
