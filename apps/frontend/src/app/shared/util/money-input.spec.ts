import { MoneyParseError, formatMoneyInput, parseMoney } from './money-input';

describe('parseMoney', () => {
  it('reads whole euros as cents', () => {
    expect(parseMoney('25')).toBe(2500);
    expect(parseMoney('2500000')).toBe(250000000);
  });

  it('reads two decimal places', () => {
    expect(parseMoney('25.50')).toBe(2550);
    expect(parseMoney('0.01')).toBe(1);
  });

  it('pads a single decimal place', () => {
    // "25.5" means twenty-five euros fifty, not twenty-five euros five cents.
    expect(parseMoney('25.5')).toBe(2550);
  });

  it('does not lose a cent to floating point', () => {
    // 25.55 * 100 is 2554.9999999999995. This is the case the string-based
    // parse exists for, so it is asserted rather than assumed.
    expect(parseMoney('25.55')).toBe(2555);
    expect(parseMoney('1.14')).toBe(114);
    expect(parseMoney('8.29')).toBe(829);
  });

  it('treats blank as undisclosed, not as zero', () => {
    // The distinction the whole model rests on: an undisclosed fee is unknown,
    // a free transfer is known to be nothing.
    expect(parseMoney('')).toBeNull();
    expect(parseMoney('   ')).toBeNull();
    expect(parseMoney(null)).toBeNull();
    expect(parseMoney(undefined)).toBeNull();
  });

  it('reads an explicit zero as a free transfer', () => {
    expect(parseMoney('0')).toBe(0);
    expect(parseMoney('0.00')).toBe(0);
  });

  it('ignores thousands separators and spaces', () => {
    expect(parseMoney('2,500,000')).toBe(250000000);
    expect(parseMoney('2 500 000.50')).toBe(250000050);
  });

  it('rejects more than two decimal places rather than rounding', () => {
    // Rounding here would be the client inventing a figure the user did not
    // type, which is precisely what the API refuses to do.
    expect(() => parseMoney('25.555')).toThrow(MoneyParseError);
  });

  it('rejects a negative amount', () => {
    expect(() => parseMoney('-5')).toThrow(MoneyParseError);
  });

  it('rejects text', () => {
    expect(() => parseMoney('abc')).toThrow(MoneyParseError);
    expect(() => parseMoney('€25')).toThrow(MoneyParseError);
    expect(() => parseMoney('25.')).toThrow(MoneyParseError);
  });

  it('rejects an amount past exact integer arithmetic', () => {
    expect(() => parseMoney('99999999999999999')).toThrow(MoneyParseError);
  });

  it('explains itself in terms the person typing can act on', () => {
    expect(() => parseMoney('25.555')).toThrow(/two decimal places/);
    expect(() => parseMoney('-5')).toThrow(/cannot be negative/);
  });
});

describe('formatMoneyInput', () => {
  it('renders cents back as a plain decimal, with no symbol', () => {
    expect(formatMoneyInput(2550)).toBe('25.50');
    expect(formatMoneyInput(250000000)).toBe('2500000.00');
  });

  it('keeps amounts below a euro readable', () => {
    expect(formatMoneyInput(1)).toBe('0.01');
    expect(formatMoneyInput(0)).toBe('0.00');
  });

  it('renders undisclosed as blank, so the field stays empty', () => {
    expect(formatMoneyInput(null)).toBe('');
    expect(formatMoneyInput(undefined)).toBe('');
  });

  it('round-trips through parseMoney', () => {
    for (const minor of [0, 1, 99, 100, 2555, 250000000]) {
      expect(parseMoney(formatMoneyInput(minor))).toBe(minor);
    }
  });
});
