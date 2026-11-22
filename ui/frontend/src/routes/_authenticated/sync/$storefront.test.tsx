import { describe, it, expect, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen } from '@/test/test-utils';
import { ConnectionToggle } from './$storefront';

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}));

describe('ConnectionToggle', () => {
  // The label is the only way to reach a store's connection card (and its
  // Disconnect), so it must be a real button a keyboard can reach and press,
  // saying whether the card is open.
  it.each([
    ['Connected', { credentialsError: false, isConfigured: true }],
    ['Not Configured', { credentialsError: false, isConfigured: false }],
    ['Credentials Error', { credentialsError: true, isConfigured: true }],
  ])('is a keyboard-operable button reading %s', async (label, props) => {
    const onToggle = vi.fn();
    const { rerender } = render(<ConnectionToggle {...props} open={false} onToggle={onToggle} />);
    const button = screen.getByRole('button', { name: label });
    expect(button).toHaveAttribute('aria-expanded', 'false');

    await userEvent.tab();
    expect(button).toHaveFocus();
    await userEvent.keyboard('{Enter}');
    expect(onToggle).toHaveBeenCalledTimes(1);

    rerender(<ConnectionToggle {...props} open onToggle={onToggle} />);
    expect(screen.getByRole('button', { name: label })).toHaveAttribute('aria-expanded', 'true');
  });
});
