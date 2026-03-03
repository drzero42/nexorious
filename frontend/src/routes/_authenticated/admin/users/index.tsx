import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/admin/users/')({
  component: () => <div>Admin Users (migrating...)</div>,
});
