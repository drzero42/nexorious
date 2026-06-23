import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useDateFormat } from './use-date-format';

const mockSettings = vi.fn();
vi.mock('./use-settings', () => ({
  useSettings: () => mockSettings(),
}));

describe('useDateFormat', () => {
  beforeEach(() => mockSettings.mockReset());

  it('binds formatters to the dmy preference', () => {
    mockSettings.mockReturnValue({ data: { dealRegion: 'us', dateFormat: 'dmy' } });
    const { result } = renderHook(() => useDateFormat());
    expect(result.current.formatDate(new Date(2026, 5, 23))).toBe('23-06-2026');
    expect(result.current.formatDateTime(new Date(2026, 5, 23, 14, 30))).toBe('23-06-2026 14:30');
  });

  it('falls back to auto when settings are not loaded', () => {
    mockSettings.mockReturnValue({ data: undefined });
    const { result } = renderHook(() => useDateFormat());
    expect(result.current.formatDate(null)).toBe('-');
    // auto path returns a non-empty locale string, not the iso literal
    expect(result.current.formatDate(new Date(2026, 5, 23))).not.toBe('2026-06-23');
  });
});
