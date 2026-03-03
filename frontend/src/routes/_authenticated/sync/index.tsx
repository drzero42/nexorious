import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/sync/')({
  component: () => <div>Sync (migrating...)</div>,
});
