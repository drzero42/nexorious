import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/games/add/confirm')({
  component: () => <div>Games Add Confirm (migrating...)</div>,
});
