import { describe, it, expect, beforeEach, vi } from 'vitest';
import { saveLibraryPrefs, loadLibraryPrefs, isEmptySearch } from './library-prefs';
import { localStorageMock } from '@/test/setup';

describe('library-prefs', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('saveLibraryPrefs writes JSON under the versioned key', () => {
    saveLibraryPrefs({ status: ['playing'], sort: 'title' });
    expect(localStorageMock.setItem).toHaveBeenCalledWith(
      'nexorious:library-view:v1',
      JSON.stringify({ status: ['playing'], sort: 'title' }),
    );
  });

  it('saveLibraryPrefs swallows write errors (e.g. quota exceeded)', () => {
    localStorageMock.setItem.mockImplementationOnce(() => {
      throw new Error('quota');
    });
    expect(() => saveLibraryPrefs({ sort: 'title' })).not.toThrow();
  });

  it('loadLibraryPrefs round-trips a saved object', () => {
    localStorageMock.getItem.mockReturnValueOnce(
      JSON.stringify({ status: ['playing'], page: '2' }),
    );
    expect(loadLibraryPrefs()).toEqual({ status: ['playing'], page: '2' });
  });

  it('loadLibraryPrefs returns null when nothing is stored', () => {
    localStorageMock.getItem.mockReturnValueOnce(null);
    expect(loadLibraryPrefs()).toBeNull();
  });

  it('loadLibraryPrefs returns null (no throw) on corrupt JSON', () => {
    localStorageMock.getItem.mockReturnValueOnce('{not valid json');
    expect(loadLibraryPrefs()).toBeNull();
  });

  it('loadLibraryPrefs returns null when stored value is not an object', () => {
    localStorageMock.getItem.mockReturnValueOnce(JSON.stringify('a string'));
    expect(loadLibraryPrefs()).toBeNull();
  });

  it('isEmptySearch distinguishes empty from populated params', () => {
    expect(isEmptySearch({})).toBe(true);
    expect(isEmptySearch({ sort: 'title' })).toBe(false);
  });
});
