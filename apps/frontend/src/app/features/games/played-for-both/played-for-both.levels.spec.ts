import { LevelWire, PlayedForBothWire } from '../../../core/models/games';
import {
  buildIndex,
  buildLevelPool,
  drawFromLevel,
  findLevel,
  matches,
  splitKey,
} from './played-for-both.engine';

const easy: LevelWire = {
  code: 'easy',
  leagues: null,
  famousOnly: true,
  clubs: ['rma', 'bar', 'mun'],
  nations: ['ESP'],
  widerChance: 0.25,
  widerLeagues: ['eng', 'esp'],
};
const medium: LevelWire = {
  code: 'medium',
  leagues: ['esp'],
  famousOnly: true,
  clubs: null,
  nations: null,
  widerChance: 0,
  widerLeagues: null,
};

// 0 rma, 1 bar, 2 mun (big); 3 bet (small, esp); 4 boc (small, arg).
const WIRE: PlayedForBothWire = {
  leagues: [
    ['eng', 'England', 'Premier League'],
    ['esp', 'Spain', 'LaLiga'],
    ['arg', 'Argentina', 'Liga Profesional'],
  ],
  clubs: [
    ['rma', 'Real Madrid', '#fff', '#febe10', 1],
    ['bar', 'Barcelona', '#a50044', '#004d98', 1],
    ['mun', 'Manchester United', '#da291c', '#000', 0],
    ['bet', 'Real Betis', '#0bb363', '#fff', 1],
    ['boc', 'Boca Juniors', '#003b94', '#f3b229', 2],
  ],
  nations: [
    ['ESP', 'Spain', 'İspanya', 'es'],
    ['ARG', 'Argentina', 'Arjantin', 'ar'],
  ],
  players: [
    ['Luis Figo', -1, [1, 0], 1],
    ['Cristiano Ronaldo', -1, [2, 0], 1],
    ['Sergio Ramos', 0, [3, 0], 1],
    ['Juan Román Riquelme', 1, [4, 1], 1],
    ['Unknown Man', -1, [1, 2], 0],
  ],
  levels: [easy, medium],
};

describe('difficulty levels', () => {
  const ix = buildIndex(WIRE);
  const names = (key: number) =>
    splitKey(key).map((e) =>
      e < ix.clubCount ? ix.clubs[e].id : ix.nations[e - ix.clubCount].fifa,
    );

  it('reads the levels from the dataset, or none from an old copy', () => {
    expect(findLevel(ix, 'easy')).toEqual(easy);
    expect(findLevel(ix, 'hard')).toBeUndefined();
    const { levels: _gone, ...old } = WIRE;
    expect(buildIndex(old).levels).toEqual([]);
  });

  it('easy pairs big clubs and allowed nations, and keeps smaller clubs aside', () => {
    const pool = buildLevelPool(ix, easy);
    expect(
      pool.core
        .map(names)
        .map((p) => p.sort().join('-'))
        .sort(),
    ).toEqual([
      'ESP-rma', // Ramos's nation, with the big club he played for
      'bar-rma',
      'mun-rma',
    ]);
    // Riquelme: Boca is not in a wider league, Argentina not an easy nation.
    expect(pool.wider.map(names).map((p) => p.sort().join('-'))).toEqual(['bet-rma']);
    expect(pool.widerChance).toBe(0.25);
  });

  it('medium takes every club of its leagues, from famous players only', () => {
    const pool = buildLevelPool(ix, medium);
    const pairs = pool.core
      .map(names)
      .map((p) => p.sort().join('-'))
      .sort();
    expect(pairs).toEqual(['ARG-bar', 'ESP-bet', 'ESP-rma', 'bar-rma', 'bet-rma']);
    expect(pairs).not.toContain('bar-mun'); // the unknown man's pair
    expect(pool.wider).toEqual([]);
  });

  it('draws a smaller club about as often as the level says', () => {
    const pool = buildLevelPool(ix, easy);
    let seed = 7;
    const rng = () => ((seed = (seed * 16807) % 2147483647) - 1) / 2147483646;
    let smaller = 0;
    for (let i = 0; i < 4000; i++) {
      const d = drawFromLevel(pool, [], rng)!;
      expect(matches(ix, d.left, d.right).length).toBeGreaterThan(0);
      if (pool.wider.includes(d.key)) smaller++;
    }
    expect(smaller / 4000).toBeGreaterThan(0.2);
    expect(smaller / 4000).toBeLessThan(0.3);
  });

  it('has nothing to draw from an empty pool', () => {
    expect(drawFromLevel({ core: [], wider: [], widerChance: 0 }, [])).toBeNull();
  });
});
