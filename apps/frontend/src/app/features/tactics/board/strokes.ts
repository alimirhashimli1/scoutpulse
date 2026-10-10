import * as THREE from 'three';

import { dashes, dist, ellipse, rect, sampleCurve, smooth, wave } from './geometry';
import { DEFAULT_STROKE, Drawing, HALF_LENGTH, HALF_WIDTH, PitchMarkings, Vec2 } from './model';
import { groundTextTexture } from './textures';

/**
 * Everything drawn flat on the grass: members' arrows, lines, zones and text,
 * and the pitch markings themselves.
 *
 * All of it is real geometry rather than paint on the grass texture, so a
 * line is as sharp zoomed in on the six-yard box as it is on the whole pitch.
 */

/** Height above the grass, so strokes never flicker into it. */
const LIFT = 0.025;

export { DEFAULT_STROKE } from './model';

/**
 * A flat ribbon along one or more polylines, built as a single geometry.
 * Joins are mitred, with the mitre capped so a hairpin turn in a pen stroke
 * does not shoot a spike across the pitch.
 */
export function ribbon(lines: Vec2[][], width: number, lift = LIFT): THREE.BufferGeometry {
  const pos: number[] = [];
  const idx: number[] = [];
  const hw = width / 2;

  for (const line of lines) {
    if (line.length < 2) continue;
    const base = pos.length / 3;
    for (let i = 0; i < line.length; i++) {
      const prev = line[Math.max(0, i - 1)];
      const next = line[Math.min(line.length - 1, i + 1)];
      const here = line[i];
      let nx: number;
      let ny: number;
      let scale = 1;
      if (i === 0 || i === line.length - 1) {
        const dx = next[0] - prev[0];
        const dy = next[1] - prev[1];
        const len = Math.hypot(dx, dy) || 1;
        nx = -dy / len;
        ny = dx / len;
      } else {
        const ax = here[0] - prev[0];
        const ay = here[1] - prev[1];
        const bx = next[0] - here[0];
        const by = next[1] - here[1];
        const la = Math.hypot(ax, ay) || 1;
        const lb = Math.hypot(bx, by) || 1;
        const n1x = -ay / la;
        const n1y = ax / la;
        const n2x = -by / lb;
        const n2y = bx / lb;
        nx = n1x + n2x;
        ny = n1y + n2y;
        const len = Math.hypot(nx, ny) || 1;
        nx /= len;
        ny /= len;
        const cos = nx * n1x + ny * n1y;
        scale = Math.min(2.5, 1 / Math.max(0.2, cos));
      }
      pos.push(here[0] + nx * hw * scale, lift, here[1] + ny * hw * scale);
      pos.push(here[0] - nx * hw * scale, lift, here[1] - ny * hw * scale);
      if (i > 0) {
        const a = base + (i - 1) * 2;
        idx.push(a, a + 1, a + 2, a + 1, a + 3, a + 2);
      }
    }
  }

  const geo = new THREE.BufferGeometry();
  geo.setAttribute('position', new THREE.Float32BufferAttribute(pos, 3));
  geo.setIndex(idx);
  geo.computeVertexNormals();
  return geo;
}

function flatMaterial(color: string, opacity = 1): THREE.MeshBasicMaterial {
  return new THREE.MeshBasicMaterial({
    color,
    transparent: true,
    opacity,
    depthWrite: false,
    side: THREE.DoubleSide,
    polygonOffset: true,
    polygonOffsetFactor: -2,
  });
}

/** A filled triangle pointing along `from → tip`. */
function arrowHead(from: Vec2, tip: Vec2, width: number, lift: number): THREE.BufferGeometry {
  const len = Math.max(1.1, width * 4.2);
  const half = Math.max(0.6, width * 2.1);
  const d = dist(from, tip) || 1;
  const ux = (tip[0] - from[0]) / d;
  const uy = (tip[1] - from[1]) / d;
  const bx = tip[0] - ux * len;
  const by = tip[1] - uy * len;
  const geo = new THREE.BufferGeometry();
  geo.setAttribute(
    'position',
    new THREE.Float32BufferAttribute(
      [
        tip[0],
        lift,
        tip[1],
        bx - uy * half,
        lift,
        by + ux * half,
        bx + uy * half,
        lift,
        by - ux * half,
      ],
      3,
    ),
  );
  return geo;
}

/** Shortens a polyline by `amount` metres at its end, so a head can sit there. */
function trimEnd(points: Vec2[], amount: number): Vec2[] {
  const out = points.slice();
  let remaining = amount;
  while (out.length > 1 && remaining > 0) {
    const a = out[out.length - 2];
    const b = out[out.length - 1];
    const d = dist(a, b);
    if (d > remaining) {
      const t = (d - remaining) / d;
      out[out.length - 1] = [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t];
      return out;
    }
    remaining -= d;
    out.pop();
  }
  return out;
}

/** The centreline a drawing follows, before styling. */
export function drawingPath(d: Drawing): Vec2[] {
  const pts = d.points;
  switch (d.kind) {
    case 'arrow':
    case 'line':
      return pts.length >= 2 ? sampleCurve(pts[0], pts[pts.length - 1], d.control, 40) : pts;
    case 'pen':
      return smooth(pts);
    case 'rect':
      return pts.length >= 2 ? rect(pts[0], pts[1]) : pts;
    case 'ellipse':
      return pts.length >= 2 ? ellipse(pts[0], pts[1]) : pts;
    case 'polygon':
      return pts.length >= 3 ? [...pts, pts[0]] : pts;
    default:
      return pts;
  }
}

/** Builds the meshes for one drawing. */
export function buildDrawing(d: Drawing, opacity = 1, layer = 0): THREE.Group {
  const g = new THREE.Group();
  g.userData['drawingId'] = d.id;
  const lift = LIFT + layer * 0.002;
  const width = d.width ?? DEFAULT_STROKE;
  const mat = flatMaterial(d.color);

  if (d.kind === 'text') {
    const at = d.points[0];
    if (!at || !d.text) return g;
    const { tex, aspect } = groundTextTexture(d.text, d.color);
    const height = 2.4 * (width / DEFAULT_STROKE);
    const plane = new THREE.Mesh(
      new THREE.PlaneGeometry(height * aspect, height),
      new THREE.MeshBasicMaterial({ map: tex, transparent: true, depthWrite: false }),
    );
    plane.rotation.x = -Math.PI / 2;
    plane.position.set(at[0], lift + 0.01, at[1]);
    g.add(plane);
    tag(g, d.id);
    setOpacity(g, opacity);
    return g;
  }

  let path = drawingPath(d);
  if (path.length < 2) return g;

  const zone = d.kind === 'rect' || d.kind === 'ellipse' || d.kind === 'polygon';
  if (zone) {
    const shape = new THREE.Shape(path.map(([x, y]) => new THREE.Vector2(x, y)));
    const fill = new THREE.Mesh(
      new THREE.ShapeGeometry(shape),
      flatMaterial(d.color, d.fill ?? 0.22),
    );
    fill.rotation.x = Math.PI / 2;
    fill.position.y = lift - 0.003;
    g.add(fill);
  }

  const headed = d.kind === 'arrow';
  let headFrom: Vec2 | undefined;
  const tip = path[path.length - 1];
  if (headed) {
    // Aim the head along the last metre of the path, not just its final
    // sample, which on a wavy line can point anywhere.
    const back = trimEnd(path, 1);
    headFrom = back[back.length - 1];
    path = trimEnd(path, Math.max(1.1, width * 4.2) * 0.7);
  }

  const style = d.style ?? 'solid';
  let lines: Vec2[][];
  if (style === 'wavy' && !zone) lines = [wave(path)];
  else if (style === 'dashed')
    lines = dashes(path, Math.max(1.2, width * 4), Math.max(0.8, width * 2.6));
  else lines = [path];

  g.add(new THREE.Mesh(ribbon(lines, width, lift), mat));
  if (headed && headFrom) g.add(new THREE.Mesh(arrowHead(headFrom, tip, width, lift + 0.001), mat));

  tag(g, d.id);
  setOpacity(g, opacity);
  return g;
}

function tag(g: THREE.Object3D, id: string): void {
  g.traverse((c) => (c.userData['drawingId'] = id));
}

// --- pitch markings ------------------------------------------------------

const L = HALF_LENGTH;
const W = HALF_WIDTH;

function arc(cx: number, cy: number, r: number, from: number, to: number, n = 48): Vec2[] {
  const out: Vec2[] = [];
  for (let i = 0; i <= n; i++) {
    const t = from + ((to - from) * i) / n;
    out.push([cx + Math.cos(t) * r, cy + Math.sin(t) * r]);
  }
  return out;
}

/** The lines of a full-size pitch, in metres, as polylines. */
function standardLines(): Vec2[][] {
  const lines: Vec2[][] = [
    [
      [-L, -W],
      [L, -W],
      [L, W],
      [-L, W],
      [-L, -W],
    ],
    [
      [0, -W],
      [0, W],
    ],
    arc(0, 0, 9.15, 0, Math.PI * 2, 96),
  ];
  for (const side of [-1, 1]) {
    const goalLine = side * L;
    const box = goalLine - side * 16.5;
    const six = goalLine - side * 5.5;
    lines.push([
      [goalLine, -20.16],
      [box, -20.16],
      [box, 20.16],
      [goalLine, 20.16],
    ]);
    lines.push([
      [goalLine, -9.16],
      [six, -9.16],
      [six, 9.16],
      [goalLine, 9.16],
    ]);
    // The D: the part of the 9.15 m circle round the spot outside the box.
    const spot = goalLine - side * 11;
    const a = Math.acos(5.5 / 9.15);
    lines.push(side < 0 ? arc(spot, 0, 9.15, -a, a) : arc(spot, 0, 9.15, Math.PI - a, Math.PI + a));
    for (const end of [-1, 1]) {
      const start = side < 0 ? (end < 0 ? 0 : -Math.PI / 2) : end < 0 ? Math.PI / 2 : Math.PI;
      lines.push(arc(goalLine, end * W, 1, start, start + Math.PI / 2, 12));
    }
  }
  return lines;
}

/** The markings as a group: lines, plus spots as small discs. */
export function buildMarkings(
  markings: PitchMarkings,
  color: string,
  opacity: number,
): THREE.Group {
  const g = new THREE.Group();
  const mat = flatMaterial(color, opacity);

  if (markings === 'standard') {
    g.add(new THREE.Mesh(ribbon(standardLines(), 0.14, 0.012), mat));
    const spot = new THREE.CircleGeometry(0.22, 20);
    for (const x of [0, -L + 11, L - 11]) {
      const s = new THREE.Mesh(spot, mat);
      s.rotation.x = -Math.PI / 2;
      s.position.set(x, 0.013, 0);
      g.add(s);
    }
  } else if (markings === 'grid') {
    g.add(
      new THREE.Mesh(
        ribbon(
          [
            [
              [-L, -W],
              [L, -W],
              [L, W],
              [-L, W],
              [-L, -W],
            ],
          ],
          0.14,
          0.012,
        ),
        mat,
      ),
    );
    const grid: Vec2[][] = [];
    for (let x = -L + 5; x < L; x += 5)
      grid.push([
        [x, -W],
        [x, W],
      ]);
    for (let y = -W + 5; y < W; y += 5)
      grid.push([
        [-L, y],
        [L, y],
      ]);
    g.add(new THREE.Mesh(ribbon(grid, 0.06, 0.011), flatMaterial(color, opacity * 0.35)));
  }
  return g;
}

export function disposeGroup(g: THREE.Object3D): void {
  g.traverse((c) => {
    if (c instanceof THREE.Mesh) {
      c.geometry.dispose();
      (c.material as THREE.Material).dispose();
    }
  });
}

/**
 * Fades a drawing without rebuilding it. Each material remembers the opacity
 * it was built with, so a zone's translucent fill fades in proportion rather
 * than jumping to opaque.
 */
export function setOpacity(g: THREE.Object3D, opacity: number): void {
  g.traverse((c) => {
    if (!(c instanceof THREE.Mesh)) return;
    const m = c.material as THREE.Material;
    if (m.userData['base'] === undefined) m.userData['base'] = m.opacity;
    m.opacity = (m.userData['base'] as number) * opacity;
  });
}
