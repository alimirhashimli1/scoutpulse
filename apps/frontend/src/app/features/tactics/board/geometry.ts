import { Vec2 } from './model';

/**
 * Plane geometry for strokes and paths, kept free of Three.js so it can be
 * tested and reused by anything that draws a board.
 */

export function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t;
}

export function lerp2(a: Vec2, b: Vec2, t: number): Vec2 {
  return [lerp(a[0], b[0], t), lerp(a[1], b[1], t)];
}

export function dist(a: Vec2, b: Vec2): number {
  return Math.hypot(b[0] - a[0], b[1] - a[1]);
}

/** A point on the quadratic curve a → b bent towards c. */
export function quad(a: Vec2, c: Vec2, b: Vec2, t: number): Vec2 {
  const u = 1 - t;
  return [
    u * u * a[0] + 2 * u * t * c[0] + t * t * b[0],
    u * u * a[1] + 2 * u * t * c[1] + t * t * b[1],
  ];
}

/**
 * The control point that makes a quadratic curve pass *through* `m` at its
 * middle. Handles are dragged to where the curve should go, which is what a
 * person expects; the raw control point sits twice as far out.
 */
export function controlThrough(a: Vec2, m: Vec2, b: Vec2): Vec2 {
  return [2 * m[0] - (a[0] + b[0]) / 2, 2 * m[1] - (a[1] + b[1]) / 2];
}

/** The curve's midpoint, which is where its handle is drawn. */
export function curveMid(a: Vec2, b: Vec2, control?: Vec2): Vec2 {
  return control ? quad(a, control, b, 0.5) : lerp2(a, b, 0.5);
}

/** Samples a straight or bent segment into a polyline. */
export function sampleCurve(a: Vec2, b: Vec2, control?: Vec2, segments = 32): Vec2[] {
  if (!control) return [a, b];
  const out: Vec2[] = [];
  for (let i = 0; i <= segments; i++) out.push(quad(a, control, b, i / segments));
  return out;
}

/** Total length of a polyline. */
export function length(points: Vec2[]): number {
  let total = 0;
  for (let i = 1; i < points.length; i++) total += dist(points[i - 1], points[i]);
  return total;
}

/**
 * Resamples a polyline at even spacing. Wavy and dashed strokes need points
 * spread by distance, not by however fast the pen happened to move.
 */
export function resample(points: Vec2[], spacing: number): Vec2[] {
  if (points.length < 2) return points.slice();
  const out: Vec2[] = [points[0]];
  let carry = 0;
  for (let i = 1; i < points.length; i++) {
    const a = points[i - 1];
    const b = points[i];
    const d = dist(a, b);
    let t = spacing - carry;
    while (t <= d) {
      out.push(lerp2(a, b, t / d));
      t += spacing;
    }
    carry = d - (t - spacing);
  }
  const last = points[points.length - 1];
  if (dist(out[out.length - 1], last) > 1e-6) out.push(last);
  return out;
}

/**
 * Offsets a polyline sideways in a sine wave: the dribble line. The wave fades
 * in and out at the ends so the arrowhead sits on the true path.
 */
export function wave(points: Vec2[], amplitude = 0.45, wavelength = 1.8): Vec2[] {
  const even = resample(points, wavelength / 8);
  const total = length(even);
  let travelled = 0;
  return even.map((p, i) => {
    if (i > 0) travelled += dist(even[i - 1], p);
    const prev = even[Math.max(0, i - 1)];
    const next = even[Math.min(even.length - 1, i + 1)];
    const dx = next[0] - prev[0];
    const dy = next[1] - prev[1];
    const len = Math.hypot(dx, dy) || 1;
    const envelope = Math.min(1, travelled / 1.5, (total - travelled) / 1.5);
    const offset =
      Math.sin((travelled / wavelength) * Math.PI * 2) * amplitude * Math.max(0, envelope);
    return [p[0] - (dy / len) * offset, p[1] + (dx / len) * offset];
  });
}

/** Splits a polyline into dashes of `on` metres separated by `off`. */
export function dashes(points: Vec2[], on = 1.4, off = 0.9): Vec2[][] {
  const even = resample(points, 0.2);
  const out: Vec2[][] = [];
  let current: Vec2[] = [];
  let travelled = 0;
  const period = on + off;
  for (let i = 0; i < even.length; i++) {
    if (i > 0) travelled += dist(even[i - 1], even[i]);
    const drawing = travelled % period < on;
    if (drawing) current.push(even[i]);
    else if (current.length) {
      if (current.length > 1) out.push(current);
      current = [];
    }
  }
  if (current.length > 1) out.push(current);
  return out;
}

/**
 * Smooths a freehand stroke with Chaikin's corner cutting. Raw pointer samples
 * are jagged at any zoom; two passes make a pen line look drawn, not plotted.
 */
export function smooth(points: Vec2[], passes = 2): Vec2[] {
  let pts = points;
  for (let n = 0; n < passes && pts.length > 2; n++) {
    const out: Vec2[] = [pts[0]];
    for (let i = 0; i < pts.length - 1; i++) {
      const a = pts[i];
      const b = pts[i + 1];
      out.push(lerp2(a, b, 0.25), lerp2(a, b, 0.75));
    }
    out.push(pts[pts.length - 1]);
    pts = out;
  }
  return pts;
}

/** Drops samples closer than `min` to the last kept one. */
export function thin(points: Vec2[], min = 0.3): Vec2[] {
  if (!points.length) return points;
  const out: Vec2[] = [points[0]];
  for (const p of points) if (dist(out[out.length - 1], p) >= min) out.push(p);
  return out;
}

/** Points around an axis-aligned ellipse inscribed in the box a–b. */
export function ellipse(a: Vec2, b: Vec2, segments = 64): Vec2[] {
  const cx = (a[0] + b[0]) / 2;
  const cy = (a[1] + b[1]) / 2;
  const rx = Math.abs(b[0] - a[0]) / 2;
  const ry = Math.abs(b[1] - a[1]) / 2;
  const out: Vec2[] = [];
  for (let i = 0; i <= segments; i++) {
    const t = (i / segments) * Math.PI * 2;
    out.push([cx + Math.cos(t) * rx, cy + Math.sin(t) * ry]);
  }
  return out;
}

export function rect(a: Vec2, b: Vec2): Vec2[] {
  return [a, [b[0], a[1]], b, [a[0], b[1]], a];
}

/** Distance from p to the segment a–b, for hit-testing thin strokes. */
export function distToSegment(p: Vec2, a: Vec2, b: Vec2): number {
  const dx = b[0] - a[0];
  const dy = b[1] - a[1];
  const len2 = dx * dx + dy * dy;
  const t = len2 ? Math.max(0, Math.min(1, ((p[0] - a[0]) * dx + (p[1] - a[1]) * dy) / len2)) : 0;
  return dist(p, [a[0] + t * dx, a[1] + t * dy]);
}

export function distToPolyline(p: Vec2, points: Vec2[]): number {
  let best = Infinity;
  for (let i = 1; i < points.length; i++)
    best = Math.min(best, distToSegment(p, points[i - 1], points[i]));
  return points.length === 1 ? dist(p, points[0]) : best;
}

export function insidePolygon(p: Vec2, poly: Vec2[]): boolean {
  let inside = false;
  for (let i = 0, j = poly.length - 1; i < poly.length; j = i++) {
    const [xi, yi] = poly[i];
    const [xj, yj] = poly[j];
    if (yi > p[1] !== yj > p[1] && p[0] < ((xj - xi) * (p[1] - yi)) / (yj - yi) + xi)
      inside = !inside;
  }
  return inside;
}

export const easeInOut = (t: number) => -(Math.cos(Math.PI * t) - 1) / 2;
export const easeOut = (t: number) => 1 - (1 - t) * (1 - t);
