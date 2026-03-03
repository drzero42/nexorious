import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/jobs/$id')({
  component: () => <div>Job Detail (migrating...)</div>,
});
