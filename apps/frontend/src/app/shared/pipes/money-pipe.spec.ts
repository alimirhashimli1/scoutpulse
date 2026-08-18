import { MoneyPipe } from './money-pipe';

describe('MoneyPipe', () => {
  let pipe: MoneyPipe;

  beforeEach(() => {
    pipe = new MoneyPipe();
  });

  describe('minor units', () => {
    it('treats the value as the currency’s smallest unit', () => {
      // 2500 minor units is €25.00, not €2,500.
      expect(pipe.transform(2500)).toBe('€25.00');
    });

    it('formats a whole amount with both decimal places', () => {
      expect(pipe.transform(100)).toBe('€1.00');
    });

    it('keeps sub-unit precision', () => {
      expect(pipe.transform(2599)).toBe('€25.99');
    });

    it('formats zero as zero, not as empty', () => {
      // A free transfer has a fee of 0. That is a fact, and different from a
      // fee that was not disclosed.
      expect(pipe.transform(0)).toBe('€0.00');
    });

    it('handles a realistic transfer fee', () => {
      expect(pipe.transform(2_500_000_000)).toBe('€25,000,000.00');
    });
  });

  describe('undisclosed amounts', () => {
    // null means undisclosed, which is not the same as free. Rendering it as
    // €0.00 would assert something the data does not say.
    it('renders null as a dash', () => {
      expect(pipe.transform(null)).toBe('—');
    });

    it('renders undefined as a dash', () => {
      expect(pipe.transform(undefined)).toBe('—');
    });

    it('accepts a caller-supplied label', () => {
      expect(pipe.transform(null, { emptyLabel: 'Undisclosed' })).toBe('Undisclosed');
    });

    it('does not render NaN', () => {
      expect(pipe.transform(Number.NaN)).toBe('—');
    });
  });

  describe('compact form', () => {
    it('abbreviates millions', () => {
      expect(pipe.transform(2_500_000_000, { compact: true })).toBe('€25.0m');
    });

    it('abbreviates thousands', () => {
      expect(pipe.transform(5_000_000, { compact: true })).toBe('€50.0k');
    });

    it('drops the decimal above 100 million', () => {
      // "€125m" reads; "€125.4m" is noise at that magnitude.
      expect(pipe.transform(12_540_000_000, { compact: true })).toBe('€125m');
    });

    it('leaves small amounts alone', () => {
      expect(pipe.transform(2500, { compact: true })).toBe('€25.00');
    });
  });

  describe('currency', () => {
    it('defaults to EUR, matching the API', () => {
      expect(pipe.transform(2500)).toContain('€');
    });

    it('honours the record’s currency', () => {
      expect(pipe.transform(2500, { currency: 'GBP' })).toBe('£25.00');
    });
  });
});
