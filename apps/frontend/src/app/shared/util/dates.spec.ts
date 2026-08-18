import { toApiDate, toDateInput, todayInput } from './dates';

describe('toApiDate', () => {
  it('turns a date input into the instant the API parses', () => {
    expect(toApiDate('1998-03-05')).toBe('1998-03-05T00:00:00Z');
  });

  it('treats blank as not supplied', () => {
    // An optional date left empty must be omitted, not sent as an empty string
    // — the API would fail to parse that and report a layout error.
    expect(toApiDate('')).toBeUndefined();
    expect(toApiDate('  ')).toBeUndefined();
    expect(toApiDate(null)).toBeUndefined();
    expect(toApiDate(undefined)).toBeUndefined();
  });

  it('does not shift the day', () => {
    // The bug this guards: `new Date('1998-03-05')` is UTC midnight, and
    // reading it back with local getters west of Greenwich gives the 4th.
    // Working on the string means there is no zone to get wrong.
    expect(toApiDate('2026-01-01')).toBe('2026-01-01T00:00:00Z');
    expect(toApiDate('2026-12-31')).toBe('2026-12-31T00:00:00Z');
  });
});

describe('toDateInput', () => {
  it('takes the calendar date out of an instant', () => {
    expect(toDateInput('1998-03-05T00:00:00Z')).toBe('1998-03-05');
  });

  it('keeps the stored day whatever the local zone is', () => {
    expect(toDateInput('2026-01-01T00:00:00Z')).toBe('2026-01-01');
  });

  it('renders a missing date as an empty field', () => {
    expect(toDateInput(null)).toBe('');
    expect(toDateInput(undefined)).toBe('');
    expect(toDateInput('')).toBe('');
  });

  it('round-trips', () => {
    expect(toApiDate(toDateInput('2024-06-30T00:00:00Z'))).toBe('2024-06-30T00:00:00Z');
  });
});

describe('todayInput', () => {
  it('zero-pads month and day', () => {
    expect(todayInput(new Date(2026, 0, 5))).toBe('2026-01-05');
  });

  it('uses the local calendar day, not the UTC one', () => {
    // 1am local on the 16th is still the 16th to the person filing it, even
    // where that is the 15th in UTC.
    const localEarlyMorning = new Date(2026, 7, 16, 1, 0, 0);
    expect(todayInput(localEarlyMorning)).toBe('2026-08-16');
  });
});
