import { controlThrough, curveMid, dashes, length, resample, wave } from './geometry';
import {
  TacticDoc,
  emptyDoc,
  formation,
  movedInto,
  normalizeDoc,
  positionAt,
  newStep,
} from './model';
import { frameAt, stepAtTime, timeline } from './playback';

/** A board with a ball and one player over three steps. */
function sample(): TacticDoc {
  const doc = emptyDoc();
  doc.objects = [
    { id: 'p', kind: 'player', team: 'home', number: '7' },
    { id: 'b', kind: 'ball' },
  ];
  doc.steps = [
    { ...newStep(), id: 's0', duration: 1, hold: 0.5, pos: { p: [0, 0], b: [1, 0] } },
    { ...newStep(), id: 's1', duration: 2, hold: 0.5, pos: { b: [21, 0] }, air: ['b'] },
    { ...newStep(), id: 's2', duration: 1, hold: 0, pos: { p: [10, 10] } },
  ];
  return doc;
}

describe('board positions', () => {
  it('carries a position forward until a later step moves it', () => {
    const doc = sample();
    expect(positionAt(doc, 1, 'p')).toEqual([0, 0]);
    expect(positionAt(doc, 2, 'p')).toEqual([10, 10]);
    expect(positionAt(doc, 2, 'b')).toEqual([21, 0]);
  });

  it('knows what moved into a step, and from where', () => {
    const doc = sample();
    expect(movedInto(doc, 1, 'b')).toEqual([1, 0]);
    expect(movedInto(doc, 1, 'p')).toBeUndefined();
    expect(movedInto(doc, 0, 'b')).toBeUndefined();
  });
});

describe('playback', () => {
  it('lays the steps out as holds and moves', () => {
    const line = timeline(sample());
    // hold 0.5, move 2 (step 2 reached at 2.5), hold 0.5, move 1 (step 3
    // reached at 4), hold 0.
    expect(line.total).toBeCloseTo(4);
    expect(line.stepStarts).toEqual([0, 2.5, 4]);
  });

  it('interpolates halfway through a move and lofts a ball marked as in the air', () => {
    const doc = sample();
    const line = timeline(doc);
    const mid = frameAt(doc, line, 0.5 + 1);
    const ball = mid.placements.get('b')!;
    expect(ball.x).toBeGreaterThan(1);
    expect(ball.x).toBeLessThan(21);
    expect(ball.h).toBeGreaterThan(1);
    // The player does not move in that step.
    expect(mid.placements.get('p')).toMatchObject({ x: 0, y: 0, h: 0 });
  });

  it('stands still on a step at the moment it is reached', () => {
    const doc = sample();
    const line = timeline(doc);
    const f = frameAt(doc, line, line.stepStarts[1]);
    expect(f.placements.get('b')).toMatchObject({ x: 21, y: 0, h: 0 });
    expect(stepAtTime(line, line.stepStarts[1])).toBe(1);
  });

  it('copes with a step that has no pause', () => {
    const doc = sample();
    const line = timeline(doc);
    const f = frameAt(doc, line, line.total);
    expect(f.placements.get('p')).toMatchObject({ x: 10, y: 10 });
  });
});

describe('geometry', () => {
  it('puts a curve handle exactly where it was dragged', () => {
    const a: [number, number] = [0, 0];
    const b: [number, number] = [10, 0];
    const control = controlThrough(a, [5, 4], b);
    const mid = curveMid(a, b, control);
    expect(mid[0]).toBeCloseTo(5);
    expect(mid[1]).toBeCloseTo(4);
  });

  it('resamples at even spacing and keeps the end point', () => {
    const even = resample(
      [
        [0, 0],
        [10, 0],
      ],
      1,
    );
    expect(even).toHaveLength(11);
    expect(even[even.length - 1]).toEqual([10, 0]);
  });

  it('dashes leave gaps, and a wave stays roughly on course', () => {
    const line: [number, number][] = [
      [0, 0],
      [20, 0],
    ];
    const parts = dashes(line, 1, 1);
    expect(parts.length).toBeGreaterThan(5);
    const wavy = wave(line);
    expect(wavy[wavy.length - 1][0]).toBeCloseTo(20);
    expect(length(wavy)).toBeGreaterThan(20);
  });
});

describe('formations', () => {
  it('puts eleven players in their own half', () => {
    const left = formation('4-3-3', 'home', 'left');
    expect(left.objects).toHaveLength(11);
    expect(Object.values(left.pos).every(([x]) => x < 0)).toBe(true);
    const right = formation('3-5-2', 'away', 'right');
    expect(Object.values(right.pos).every(([x]) => x > 0)).toBe(true);
    expect(right.objects.filter((o) => o.keeper)).toHaveLength(1);
  });
});

describe('normalizeDoc', () => {
  it('opens anything as a usable board', () => {
    expect(normalizeDoc(null).steps).toHaveLength(1);
    expect(normalizeDoc('nonsense').objects).toEqual([]);
    const repaired = normalizeDoc({
      steps: [{ pos: 'bad' }],
      objects: [{ nope: 1 }, { id: 'x', kind: 'cone' }],
    });
    expect(repaired.steps[0].pos).toEqual({});
    expect(repaired.objects.map((o) => o.id)).toEqual(['x']);
    expect(repaired.teams.home.primary).toBeTruthy();
  });
});
