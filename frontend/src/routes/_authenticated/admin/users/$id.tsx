import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/admin/users/$id')({
  component: () => <div>Admin User Detail (migrating...)</div>,
});
