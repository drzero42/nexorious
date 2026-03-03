import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/games/add')({
  component: () => <div>Games Add (migrating...)</div>,
});
