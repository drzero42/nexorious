import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/jobs/')({
  component: () => <div>Jobs (migrating...)</div>,
});
