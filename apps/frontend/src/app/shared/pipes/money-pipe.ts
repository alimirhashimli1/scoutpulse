import { Pipe, PipeTransform } from '@angular/core';

import { MinorUnits } from '../../core/models/football';

export interface MoneyOptions {
  /** ISO code from the record. Defaults to EUR, matching the API. */
  currency?: string;
  /**
   * Abbreviate large amounts — €25.0m rather than €25,000,000.00.
   *
   * Transfer fees are usually read at a glance and compared, not audited, so
   * the compact form is the right default for feeds and cards. Use the full
   * form where the exact figure matters.
   */
  compact?: boolean;
  /** Shown when the amount is null. A null fee means *undisclosed*. */
  emptyLabel?: string;
}

/**
 * Formats money held as an integer count of minor units.
 *
 * The single place minor units become a decimal. Doing this inline in a
 * template invites `value / 100` scattered through the app, and that is how a
 * fee ends up rendered a hundred times too large in one view and not another.
 *
 * A null amount is *undisclosed*, which is a different fact from a free
 * transfer (0) — so it renders as a dash rather than as zero.
 */
@Pipe({ name: 'money' })
export class MoneyPipe implements PipeTransform {
  transform(minor: MinorUnits | null | undefined, options: MoneyOptions = {}): string {
    const { currency = 'EUR', compact = false, emptyLabel = '—' } = options;

    if (minor === null || minor === undefined || !Number.isFinite(minor)) {
      return emptyLabel;
    }

    const major = minor / 100;

    if (compact) {
      const abs = Math.abs(major);
      if (abs >= 1_000_000_000) return this.abbreviate(major, 1_000_000_000, 'bn', currency);
      if (abs >= 1_000_000) return this.abbreviate(major, 1_000_000, 'm', currency);
      if (abs >= 1_000) return this.abbreviate(major, 1_000, 'k', currency);
    }

    return new Intl.NumberFormat('en-GB', {
      style: 'currency',
      currency,
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(major);
  }

  private abbreviate(major: number, divisor: number, suffix: string, currency: string): string {
    const scaled = major / divisor;
    // One decimal below 100, none above: "€8.5m" is useful, "€125.4m" is noise.
    const digits = Math.abs(scaled) < 100 ? 1 : 0;

    const formatted = new Intl.NumberFormat('en-GB', {
      style: 'currency',
      currency,
      minimumFractionDigits: digits,
      maximumFractionDigits: digits,
    }).format(scaled);

    return `${formatted}${suffix}`;
  }
}
