/**
 * State threaded through {@link nextTitleWrite} across navigations.
 * `logical` is the last *intended* title (without any forcing marker);
 * `flip` alternates the invisible marker so repeated titles still differ.
 */
export interface TitleWriteState {
  logical: string | null;
  flip: boolean;
}

export const initialTitleWriteState: TitleWriteState = { logical: null, flip: false };

/**
 * Compute the value to assign to `document.title`, guaranteeing it differs from
 * the previously-assigned value whenever the desired title is unchanged.
 *
 * Firefox for Android (Fenix) only repaints the tab title when `document.title`
 * changes to a *different* value after a history navigation; re-writing the same
 * string is ignored and the tab keeps showing the URL
 * (https://github.com/mozilla-mobile/fenix/issues/22667). Many library
 * navigations (sort direction, page, search) keep the same title, so when the
 * desired title repeats we alternate an invisible trailing space to force a real
 * change. The trailing space is collapsed in the tab, so it is never visible.
 */
export function nextTitleWrite(
  desired: string,
  prev: TitleWriteState,
): { value: string; state: TitleWriteState } {
  if (desired === prev.logical) {
    const flip = !prev.flip;
    return { value: flip ? `${desired} ` : desired, state: { logical: desired, flip } };
  }
  return { value: desired, state: { logical: desired, flip: false } };
}
