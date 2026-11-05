// The real dataset, from where football-svc embeds it: these specs are about
// the rules applied to the data the game actually ships.
import dataset from '../../../../../../football-svc/internal/games/playedforboth/data/dataset.json';

import { PlayedForBothWire } from '../../../core/models/games';
import {
  GameIndex,
  RECENT_WINDOW,
  TOP5,
  buildIndex,
  buildPairPool,
  drawPair,
  enabledClubs,
  isClub,
  matches,
  pairKey,
  pushRecent,
  shortCode,
  splitKey,
} from './played-for-both.engine';

/** Small deterministic PRNG (mulberry32), so failures reproduce. */
function seeded(seed: number): () => number {
  let a = seed;
  return () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

describe('played-for-both engine', () => {
  let ix: GameIndex;
  const all = () => new Set(ix.leagues.map((l) => l.code));
  const club = (name: string) => {
    const i = ix.clubs.findIndex((c) => c.name === name);
    expect(i, name).toBeGreaterThanOrEqual(0);
    return i;
  };
  const nation = (name: string) => {
    const i = ix.nations.findIndex((n) => n.nameEN === name);
    expect(i, name).toBeGreaterThanOrEqual(0);
    return ix.clubCount + i;
  };
  const names = (a: number, b: number) => matches(ix, a, b).map((p) => p.name);

  beforeAll(() => {
    ix = buildIndex(dataset as unknown as PlayedForBothWire);
  });

  it('indexes the whole dataset', () => {
    expect(ix.leagues.length).toBe(20);
    expect(ix.clubs.length).toBe(359);
    expect(ix.nations.length).toBe(60);
    expect(ix.players.length).toBeGreaterThan(11_000);
    expect(ix.byEntity.length).toBe(359 + 60);
  });

  describe('matches', () => {
    it('Togo + Tottenham is exactly Adebayor', () => {
      expect(names(nation('Togo'), club('Tottenham Hotspur'))).toEqual(['Emmanuel Adebayor']);
    });

    it('Real Madrid + PSG includes Mbappé, Ramos and Beckham', () => {
      expect(names(club('Real Madrid'), club('Paris Saint-Germain'))).toEqual(
        expect.arrayContaining(['Kylian Mbappé', 'Sergio Ramos', 'David Beckham']),
      );
    });

    it('Galatasaray + Inter includes Sneijder and Hakan Şükür', () => {
      expect(names(club('Galatasaray'), club('Inter Milan'))).toEqual(
        expect.arrayContaining(['Wesley Sneijder', 'Hakan Şükür']),
      );
    });

    it('lists famous players first, then alphabetically', () => {
      const list = matches(ix, club('Real Madrid'), club('Paris Saint-Germain'));
      const firstMinor = list.findIndex((p) => !p.famous);
      if (firstMinor >= 0) expect(list.slice(firstMinor).every((p) => !p.famous)).toBe(true);
      const famous = list.filter((p) => p.famous).map((p) => p.name);
      expect(famous).toEqual([...famous].sort((a, b) => a.localeCompare(b)));
    });

    it('is symmetric', () => {
      const a = club('Galatasaray');
      const b = club('Inter Milan');
      expect(names(a, b).sort()).toEqual(names(b, a).sort());
    });
  });

  describe('pair pool', () => {
    it('only holds pairs with a known answer', () => {
      const pool = buildPairPool(ix, { leagues: all(), famousOnly: false, allowNations: true });
      expect(pool.length).toBeGreaterThan(1000);
      for (const key of pool) {
        const [a, b] = splitKey(key);
        expect(matches(ix, a, b).length, `${a}-${b}`).toBeGreaterThan(0);
      }
    });

    it('builds famous-only pairs from famous players only', () => {
      const pool = buildPairPool(ix, { leagues: all(), famousOnly: true, allowNations: true });
      const full = buildPairPool(ix, { leagues: all(), famousOnly: false, allowNations: true });
      expect(pool.length).toBeLessThan(full.length);
      for (const key of pool) {
        const [a, b] = splitKey(key);
        expect(
          matches(ix, a, b).some((p) => p.famous),
          `${a}-${b}`,
        ).toBe(true);
      }
    });

    it('yields only club–club pairs with countries off', () => {
      const pool = buildPairPool(ix, { leagues: all(), famousOnly: true, allowNations: false });
      for (const key of pool) {
        const [a, b] = splitKey(key);
        expect(isClub(ix, a) && isClub(ix, b)).toBe(true);
      }
    });

    it('never contains nation–nation or a team against itself', () => {
      const pool = buildPairPool(ix, { leagues: all(), famousOnly: false, allowNations: true });
      for (const key of pool) {
        const [a, b] = splitKey(key);
        expect(a).not.toBe(b);
        expect(isClub(ix, a)).toBe(true);
      }
    });
  });

  describe('drawPair', () => {
    function drawMany(knownOnly: boolean, allowNations: boolean, leagues: Set<string>, n = 1000) {
      const rng = seeded(knownOnly ? 7 : 11);
      const clubs = enabledClubs(ix, leagues);
      const pool = buildPairPool(ix, { leagues, famousOnly: true, allowNations });
      let recent: number[] = [];
      const out = [];
      for (let i = 0; i < n; i++) {
        const d = drawPair(ix, { clubs, pool, knownOnly, allowNations, recent }, rng)!;
        recent = pushRecent(recent, d.key);
        out.push(d);
      }
      return out;
    }

    for (const knownOnly of [true, false]) {
      it(`over 1,000 draws (${knownOnly ? 'known answers' : 'fully random'}) keeps every rule`, () => {
        const leagues = new Set(TOP5);
        const enabled = new Set(enabledClubs(ix, leagues));
        for (const d of drawMany(knownOnly, true, leagues)) {
          expect(d.left).not.toBe(d.right);
          // never nation–nation, and any club is from an enabled league
          expect(isClub(ix, d.left) || isClub(ix, d.right)).toBe(true);
          for (const e of [d.left, d.right]) {
            if (isClub(ix, e)) expect(enabled.has(e)).toBe(true);
          }
          expect(pairKey(d.left, d.right)).toBe(d.key);
        }
      });
    }

    it('puts each team on either side', () => {
      const draws = drawMany(true, true, all(), 400);
      const swapped = draws.filter((d) => d.left > d.right).length;
      expect(swapped).toBeGreaterThan(100);
      expect(swapped).toBeLessThan(300);
    });

    it('gives only club–club pairs in random mode with countries off', () => {
      for (const d of drawMany(false, false, all(), 500)) {
        expect(isClub(ix, d.left) && isClub(ix, d.right)).toBe(true);
      }
    });

    it('does not repeat a pair within the recent window when the pool is big enough', () => {
      const draws = drawMany(true, true, all(), 600);
      for (let i = 0; i < draws.length; i++) {
        const window = draws.slice(Math.max(0, i - RECENT_WINDOW), i).map((d) => d.key);
        expect(window).not.toContain(draws[i].key);
      }
    });

    it('returns null when no league is enabled', () => {
      const d = drawPair(ix, {
        clubs: [],
        pool: [],
        knownOnly: true,
        allowNations: true,
        recent: [],
      });
      expect(d).toBeNull();
    });
  });

  it('keeps only the newest recent pairs', () => {
    let recent: number[] = [];
    for (let i = 0; i < RECENT_WINDOW + 10; i++) recent = pushRecent(recent, i);
    expect(recent.length).toBe(RECENT_WINDOW);
    expect(recent[0]).toBe(10);
  });

  it('makes badge codes', () => {
    expect(shortCode({ id: 'rma', name: 'Real Madrid' })).toBe('RMA');
    expect(shortCode({ id: 'gal', name: 'Galatasaray' })).toBe('GS');
    expect(shortCode({ id: 'xxx', name: 'Real Betis Balompié' })).toBe('BB');
    expect(shortCode({ id: 'xxx', name: 'FC Twente' })).toBe('TWE');
    expect(shortCode({ id: 'xxx', name: 'Werder Bremen' })).toBe('WB');
  });
});
