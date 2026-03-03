import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/tags')({
  component: () => <div>Tags (migrating...)</div>,
});
