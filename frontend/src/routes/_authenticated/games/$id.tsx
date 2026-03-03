import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/games/$id')({
  component: () => <div>Game Detail (migrating...)</div>,
});
