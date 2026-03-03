import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/games/$id/edit')({
  component: () => <div>Game Edit (migrating...)</div>,
});
