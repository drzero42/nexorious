import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/admin/maintenance')({
  component: () => <div>Admin Maintenance (migrating...)</div>,
});
