import { createFileRoute, Outlet } from '@tanstack/react-router';

// Re-exported for use by add.confirm.tsx
export { SELECTED_GAME_STORAGE_KEY } from './add.index';

export const Route = createFileRoute('/_authenticated/games/add')({
  component: () => <Outlet />,
});
