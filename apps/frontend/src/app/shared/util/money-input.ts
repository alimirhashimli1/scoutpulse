import { MinorUnits } from '../../core/models/football';

/**
 * Reading money *in*. The inverse of MoneyPipe, and the harder direction.
 *
 * A user types euros; the API wants an integer count of cents and rejects
 * anything else outright — it will not round 1.5 cents, because it cannot know
 * whether that meant one or two.
 *
 * The obvious `Math.round(parseFloat(text) * 100)` is wrong in a way that is
 * hard to see: `25.55 * 100` is `2554.9999999999995` in binary floating point,
 * and rounding rescues that case but not every one. Parsing the digits either
 * side of the point as integers never touches a float at all.
 */

export class MoneyParseError extends Error {}

/**
 * `"25.50"` → `2550`. An empty string is `null`, which the API reads as
 * *undisclosed* — a different fact from a free transfer, which is `0`.
 *
 * @throws MoneyParseError with a message written for the person who typed it.
 */
export function parseMoney(text: string | null | undefined): MinorUnits | null {
  const raw = text?.trim().replace(/[\s,]/g, '');
  if (!raw) return null;

  const match = /^(-?)(\d+)(?:\.(\d+))?$/.exec(raw);
  if (!match) {
    throw new MoneyParseError('Enter an amount, like 2500000 or 2500000.50');
  }

  const [, sign, whole, fraction = ''] = match;
  if (sign === '-') {
    throw new MoneyParseError('An amount cannot be negative.');
  }
  if (fraction.length > 2) {
    throw new MoneyParseError('At most two decimal places.');
  }

  const minor = Number(`${whole}${fraction.padEnd(2, '0')}`);

  // Above 2^53 the arithmetic in this app stops being exact. The API accepts a
  // quoted amount for exactly this reason, but nothing in football costs that,
  // and a silently wrong fee is worse than a refusal.
  if (!Number.isSafeInteger(minor)) {
    throw new MoneyParseError('That amount is too large.');
  }

  return minor;
}

/** `2550` → `"25.50"`, for prefilling an edit form. Never a currency symbol. */
export function formatMoneyInput(minor: MinorUnits | null | undefined): string {
  if (minor === null || minor === undefined || !Number.isFinite(minor)) return '';

  const negative = minor < 0;
  const digits = `${Math.abs(minor)}`.padStart(3, '0');
  const whole = digits.slice(0, -2);
  const fraction = digits.slice(-2);

  return `${negative ? '-' : ''}${whole}.${fraction}`;
}
