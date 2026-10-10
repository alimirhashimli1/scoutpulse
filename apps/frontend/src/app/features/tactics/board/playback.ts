import { dist, easeInOut, easeOut, lerp2, quad } from './geometry';
import { Drawing, TacticDoc, Vec2, positionAt } from './model';

/**
 * Turns a board's steps into motion.
 *
 * A board is a list of steps, each a set of positions. Playing it is a
 * sequence of holds and moves: pause on step 0, travel to step 1, pause, and
 * so on. Everything here is a pure function of the document and a time, so
 * scrubbing, playing and recording a video all draw the same frame for the
 * same moment.
 */

export interface Segment {
  kind: 'hold' | 'move';
  /** The step being held, or moved into. */
  step: number;
  start: number;
  end: number;
}

export interface Timeline {
  segments: Segment[];
  total: number;
  /** When each step is reached, for the scrubber's tick marks. */
  stepStarts: number[];
}

export function timeline(doc: TacticDoc): Timeline {
  const segments: Segment[] = [];
  const stepStarts: number[] = [];
  let t = 0;
  doc.steps.forEach((step, i) => {
    if (i > 0) {
      const d = Math.max(0.1, step.duration);
      segments.push({ kind: 'move', step: i, start: t, end: t + d });
      t += d;
    }
    stepStarts.push(t);
    const h = Math.max(0, step.hold);
    segments.push({ kind: 'hold', step: i, start: t, end: t + h });
    t += h;
  });
  return { segments, total: t, stepStarts };
}

/** Where one object is in a frame. `h` is height above the grass. */
export interface Placement {
  x: number;
  y: number;
  h: number;
  /** 0–1: fades in an object that first appears in this step. */
  appear: number;
  /** Direction of travel, radians, when it is moving. */
  heading?: number;
}

export interface Frame {
  /** The step whose drawings and caption are showing. */
  step: number;
  placements: Map<string, Placement>;
  drawings: Drawing[];
  drawingOpacity: number;
  highlights: string[];
  note: string;
}

const BALL_KINDS = new Set(['ball']);

export function frameAt(doc: TacticDoc, line: Timeline, time: number): Frame {
  const t = Math.max(0, Math.min(line.total, time));
  // The last segment that has started. A zero-length hold still counts, and
  // at a boundary the later segment wins, so the end of a move shows the
  // step it arrived at.
  let seg: Segment | undefined;
  for (const s of line.segments) if (s.start <= t + 1e-9) seg = s;

  if (!seg || seg.kind === 'hold') {
    const step = seg?.step ?? 0;
    const elapsed = seg ? t - seg.start : 0;
    return {
      step,
      placements: still(doc, step),
      drawings: doc.steps[step]?.drawings ?? [],
      // Fade the drawings in at the start of the hold, so arrows arrive after
      // the players do rather than popping in under them.
      drawingOpacity: step === 0 ? 1 : Math.min(1, elapsed / 0.3),
      highlights: doc.steps[step]?.highlight ?? [],
      note: doc.steps[step]?.note ?? '',
    };
  }

  const into = seg.step;
  const p = (t - seg.start) / (seg.end - seg.start);
  const target = doc.steps[into];
  const placements = new Map<string, Placement>();

  for (const obj of doc.objects) {
    const to = positionAt(doc, into, obj.id);
    if (!to) continue;
    const from = positionAt(doc, into - 1, obj.id);
    if (!from) {
      placements.set(obj.id, { x: to[0], y: to[1], h: 0, appear: easeOut(p) });
      continue;
    }

    const isBall = BALL_KINDS.has(obj.kind);
    const e = isBall ? easeOut(p) : easeInOut(p);
    const bend = target.bend?.[obj.id];
    const at: Vec2 = bend ? quad(from, bend, to, e) : lerp2(from, to, e);

    let h = 0;
    if (target.air?.includes(obj.id)) {
      // A lofted pass: a parabola whose peak scales with distance, as a real
      // one does, capped so a 60-metre switch does not leave the stadium.
      const peak = Math.min(14, dist(from, to) * 0.22);
      h = 4 * peak * p * (1 - p);
    }

    const ahead: Vec2 = bend ? quad(from, bend, to, Math.min(1, e + 0.02)) : to;
    const moving = dist(at, ahead) > 1e-4;
    placements.set(obj.id, {
      x: at[0],
      y: at[1],
      h,
      appear: 1,
      heading: moving ? Math.atan2(ahead[1] - at[1], ahead[0] - at[0]) : undefined,
    });
  }

  return {
    step: into - 1,
    placements,
    // The arrows from the previous step describe this move, so they stay up
    // while it happens and fade as it completes.
    drawings: doc.steps[into - 1]?.drawings ?? [],
    drawingOpacity: 1 - easeInOut(p),
    highlights: target.highlight ?? [],
    note: target.note,
  };
}

/** Every object standing still at a step. */
export function still(doc: TacticDoc, step: number): Map<string, Placement> {
  const out = new Map<string, Placement>();
  for (const obj of doc.objects) {
    const p = positionAt(doc, step, obj.id);
    if (p) out.set(obj.id, { x: p[0], y: p[1], h: 0, appear: 1 });
  }
  return out;
}

/** Which step is current at a time: the one being held or moved into. */
export function stepAtTime(line: Timeline, time: number): number {
  let current = 0;
  line.stepStarts.forEach((start, i) => {
    if (time >= start - 1e-6) current = i;
  });
  return current;
}
