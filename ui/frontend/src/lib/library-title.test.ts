import { describe, it, expect } from 'vitest';
import { buildLibraryTitle } from './library-title';

describe('buildLibraryTitle', () => {
  it('is just "Library" with no filters and default sort', () => {
    expect(buildLibraryTitle({}, 'title', 'asc')).toBe('Library | Nexorious');
  });

  it('summarises a single status filter', () => {
    expect(buildLibraryTitle({ status: ['not_started'] }, 'title', 'asc')).toBe(
      'Not Started — Library | Nexorious',
    );
  });

  it('uses platform display names from the lookup', () => {
    const title = buildLibraryTitle(
      { status: ['not_started'], platforms: ['pc-windows'] },
      'title',
      'asc',
      { platformLabels: { 'pc-windows': 'PC' } },
    );
    expect(title).toBe('Not Started, PC — Library | Nexorious');
  });

  it('falls back to a humanised value when a platform label is missing', () => {
    expect(buildLibraryTitle({ platforms: ['pc-windows'] }, 'title', 'asc')).toBe(
      'Pc Windows — Library | Nexorious',
    );
  });

  it('appends a non-default sort after the filters', () => {
    expect(buildLibraryTitle({ status: ['not_started'] }, 'rating_average', 'desc')).toBe(
      'Not Started · by IGDB Rating — Library | Nexorious',
    );
  });

  it('shows sort alone when there are no filters', () => {
    expect(buildLibraryTitle({}, 'rating_average', 'desc')).toBe(
      'by IGDB Rating — Library | Nexorious',
    );
  });

  it('omits the default sort (title/asc)', () => {
    expect(buildLibraryTitle({ status: ['completed'] }, 'title', 'asc')).toBe(
      'Completed — Library | Nexorious',
    );
  });

  it('treats a non-default direction on the default field as a sort', () => {
    expect(buildLibraryTitle({}, 'title', 'desc')).toBe('by Title — Library | Nexorious');
  });

  it('includes the loved filter', () => {
    expect(buildLibraryTitle({ isLoved: true }, 'title', 'asc')).toBe(
      'Loved — Library | Nexorious',
    );
    expect(buildLibraryTitle({ isLoved: false }, 'title', 'asc')).toBe(
      'Not loved — Library | Nexorious',
    );
  });

  it('caps the number of labels and reports the remainder', () => {
    const title = buildLibraryTitle(
      { genres: ['Action', 'RPG', 'Strategy', 'Puzzle', 'Racing', 'Sports'] },
      'title',
      'asc',
    );
    expect(title).toBe('Action, RPG, Strategy, Puzzle, +2 more — Library | Nexorious');
  });
});
