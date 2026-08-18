/**
 * Converting between what `<input type="date">` speaks and what the API does.
 *
 * The API's date fields are Go `time.Time` values behind a `Date` alias, so
 * they marshal as RFC-3339 instants — `1998-03-05T00:00:00Z` — even though
 * every one of them is a calendar date. Posting the bare `1998-03-05` that a
 * date input produces is **rejected**, with a parse error naming a layout
 * string. That is not a message anyone can act on, so the conversion happens
 * here rather than being rediscovered per form.
 *
 * Both directions work on the string, never through `new Date()`. A Date
 * parses `1998-03-05` as UTC midnight and `getDate()` reads it back in local
 * time, so anywhere west of Greenwich a birth date silently becomes the 4th.
 * String slicing has no timezone to be wrong about.
 */

/** `2026-08-16` → `2026-08-16T00:00:00Z`. Blank input means "not supplied". */
export function toApiDate(input: string | null | undefined): string | undefined {
  const value = input?.trim();
  if (!value) return undefined;
  return `${value}T00:00:00Z`;
}

/** `1998-03-05T00:00:00Z` → `1998-03-05`, for prefilling an edit form. */
export function toDateInput(iso: string | null | undefined): string {
  if (!iso) return '';
  return iso.slice(0, 10);
}

/** Today, as a date input wants it. The default for "when did this happen". */
export function todayInput(now: Date = new Date()): string {
  // Local rather than UTC: someone filing a transfer at 01:00 in Istanbul
  // means today where they are, not yesterday in Greenwich.
  const month = `${now.getMonth() + 1}`.padStart(2, '0');
  const day = `${now.getDate()}`.padStart(2, '0');
  return `${now.getFullYear()}-${month}-${day}`;
}
