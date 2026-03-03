import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/admin/platforms')({
  component: () => <div>Admin Platforms (migrating...)</div>,
});
