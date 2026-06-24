import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from '@testing-library/react';
import { ThemeSync } from './theme-sync';

const setTheme = vi.fn();
let mockTheme = 'system';
let mockSettings: { theme?: string } | undefined = undefined;

vi.mock('next-themes', () => ({
  useTheme: () => ({ theme: mockTheme, setTheme }),
}));
vi.mock('@/hooks', () => ({
  useSettings: () => ({ data: mockSettings }),
}));

describe('ThemeSync', () => {
  beforeEach(() => {
    setTheme.mockClear();
    mockTheme = 'system';
    mockSettings = undefined;
  });

  it('pushes the server theme into next-themes when they differ', () => {
    mockSettings = { theme: 'dark' };
    render(<ThemeSync />);
    expect(setTheme).toHaveBeenCalledWith('dark');
  });

  it('is a no-op when server and next-themes already match', () => {
    mockTheme = 'dark';
    mockSettings = { theme: 'dark' };
    render(<ThemeSync />);
    expect(setTheme).not.toHaveBeenCalled();
  });

  it('does nothing before settings load', () => {
    mockSettings = undefined;
    render(<ThemeSync />);
    expect(setTheme).not.toHaveBeenCalled();
  });
});
