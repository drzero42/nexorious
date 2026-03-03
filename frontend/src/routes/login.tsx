import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/login')({
  component: () => <div>Login (migrating...)</div>,
});
