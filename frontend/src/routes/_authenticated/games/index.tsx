import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated/games/')({
  component: () => <div>Games (migrating...)</div>,
});
