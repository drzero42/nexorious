import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/admin/users/new')({
  component: () => <div>Admin Users New (migrating...)</div>,
});
