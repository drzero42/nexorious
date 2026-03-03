import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/admin/backups')({
  component: () => <div>Admin Backups (migrating...)</div>,
});
