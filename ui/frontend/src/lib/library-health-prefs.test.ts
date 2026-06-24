import { describe, it, expect, beforeEach, vi } from 'vitest';
import { saveExpandedChecks, loadExpandedChecks } from './library-health-prefs';
import { sessionStorageMock } from '@/test/setup';

describe('library-health-prefs', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('saveExpandedChecks writes a JSON array under the versioned key', () => {
    saveExpandedChecks(['wishlisted-yet-owned', 'played-but-not-started']);
    expect(sessionStorageMock.setItem).toHaveBeenCalledWith(
      'nexorious:library-health-expanded:v1',
      JSON.stringify(['wishlisted-yet-owned', 'played-but-not-started']),
    );
  });

  it('saveExpandedChecks swallows write errors (e.g. quota exceeded)', () => {
    sessionStorageMock.setItem.mockImplementationOnce(() => {
      throw new Error('quota');
    });
    expect(() => saveExpandedChecks(['x'])).not.toThrow();
  });

  it('loadExpandedChecks round-trips a saved array', () => {
    sessionStorageMock.getItem.mockReturnValueOnce(JSON.stringify(['a', 'b']));
    expect(loadExpandedChecks()).toEqual(['a', 'b']);
  });

  it('loadExpandedChecks returns an empty array when nothing is stored', () => {
    sessionStorageMock.getItem.mockReturnValueOnce(null);
    expect(loadExpandedChecks()).toEqual([]);
  });

  it('loadExpandedChecks returns an empty array (no throw) on corrupt JSON', () => {
    sessionStorageMock.getItem.mockReturnValueOnce('[not valid json');
    expect(loadExpandedChecks()).toEqual([]);
  });

  it('loadExpandedChecks returns an empty array when stored value is not an array', () => {
    sessionStorageMock.getItem.mockReturnValueOnce(JSON.stringify({ a: 1 }));
    expect(loadExpandedChecks()).toEqual([]);
  });

  it('loadExpandedChecks drops non-string entries', () => {
    sessionStorageMock.getItem.mockReturnValueOnce(JSON.stringify(['a', 2, null, 'b']));
    expect(loadExpandedChecks()).toEqual(['a', 'b']);
  });
});
