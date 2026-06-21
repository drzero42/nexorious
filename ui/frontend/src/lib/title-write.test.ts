import { describe, it, expect } from 'vitest';
import { nextTitleWrite, initialTitleWriteState, type TitleWriteState } from './title-write';

describe('nextTitleWrite', () => {
  it('writes the desired title verbatim when it changes', () => {
    const { value, state } = nextTitleWrite('Library | Nexorious', initialTitleWriteState);
    expect(value).toBe('Library | Nexorious');
    expect(state).toEqual({ logical: 'Library | Nexorious', flip: false });
  });

  it('forces a different value when the desired title repeats', () => {
    // First write.
    const first = nextTitleWrite('Library | Nexorious', initialTitleWriteState);
    // Same desired title again (e.g. a sort-only navigation) must produce a
    // value distinct from the previously-written one so Firefox Android repaints.
    const second = nextTitleWrite('Library | Nexorious', first.state);
    expect(second.value).not.toBe(first.value);
    // The marker is an invisible trailing space, so the trimmed value is unchanged.
    expect(second.value.trim()).toBe('Library | Nexorious');
  });

  it('keeps every consecutive write distinct across many repeats', () => {
    let state: TitleWriteState = initialTitleWriteState;
    let prev: string | null = null;
    for (let i = 0; i < 6; i++) {
      const { value, state: next } = nextTitleWrite('Library | Nexorious', state);
      expect(value).not.toBe(prev);
      expect(value.trim()).toBe('Library | Nexorious');
      prev = value;
      state = next;
    }
  });

  it('resets the marker when the title genuinely changes', () => {
    const a = nextTitleWrite('Library | Nexorious', initialTitleWriteState);
    const b = nextTitleWrite('Library | Nexorious', a.state); // forced -> 'Library | Nexorious '
    const c = nextTitleWrite('Not Started — Library | Nexorious', b.state);
    expect(c.value).toBe('Not Started — Library | Nexorious');
    expect(c.state.flip).toBe(false);
  });
});
