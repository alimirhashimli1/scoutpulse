import * as THREE from 'three';

import { HALF_LENGTH, HALF_WIDTH, KitPattern, PitchStyle, RUNOFF } from './model';

/**
 * Textures painted on a canvas at runtime.
 *
 * Nothing here is downloaded: the grass, the kits, the ball and the nets are
 * drawn in code. That keeps the feature to a single lazy script, works offline
 * once loaded, and lets every kit colour a member picks be exact rather than
 * tinted from a stock image.
 */

const cache = new Map<string, THREE.Texture>();

function cached(key: string, make: () => THREE.Texture): THREE.Texture {
  let tex = cache.get(key);
  if (!tex) {
    tex = make();
    cache.set(key, tex);
  }
  return tex;
}

export function disposeTextures(): void {
  cache.forEach((t) => t.dispose());
  cache.clear();
}

function canvas(w: number, h: number): [HTMLCanvasElement, CanvasRenderingContext2D] {
  const c = document.createElement('canvas');
  c.width = w;
  c.height = h;
  return [c, c.getContext('2d')!];
}

function texture(c: HTMLCanvasElement, srgb = true): THREE.CanvasTexture {
  const t = new THREE.CanvasTexture(c);
  if (srgb) t.colorSpace = THREE.SRGBColorSpace;
  t.anisotropy = 8;
  return t;
}

/** A tiny deterministic PRNG, so the grass looks the same on every render. */
function rng(seed: number): () => number {
  let s = seed >>> 0;
  return () => {
    s = (s * 1664525 + 1013904223) >>> 0;
    return s / 4294967296;
  };
}

/**
 * The playing surface, runoff included: mown stripes along the length, a
 * fainter cross-cut, and per-pixel noise so it reads as grass rather than
 * paint when the camera is low.
 */
export function grassTexture(style: PitchStyle): THREE.Texture {
  return cached(`grass:${style}`, () => {
    const totalL = (HALF_LENGTH + RUNOFF) * 2;
    const totalW = (HALF_WIDTH + RUNOFF) * 2;
    const W = 2048;
    const H = Math.round((W * totalW) / totalL);
    const [c, g] = canvas(W, H);
    const pxPerM = W / totalL;

    const palette =
      style === 'board'
        ? { a: '#1f4d3a', b: '#1f4d3a', run: '#1b4433', noise: 0.05 }
        : style === 'night'
          ? { a: '#2c7a3c', b: '#246b33', run: '#1f5c2c', noise: 0.08 }
          : { a: '#3f9b4c', b: '#348a41', run: '#2f7d3b', noise: 0.08 };

    g.fillStyle = palette.run;
    g.fillRect(0, 0, W, H);

    // Stripes across the pitch proper, eighteen of them as on most grounds.
    const x0 = RUNOFF * pxPerM;
    const y0 = RUNOFF * pxPerM;
    const pitchW = HALF_LENGTH * 2 * pxPerM;
    const pitchH = HALF_WIDTH * 2 * pxPerM;
    const stripes = 18;
    for (let i = 0; i < stripes; i++) {
      g.fillStyle = i % 2 ? palette.a : palette.b;
      g.fillRect(x0 + (i * pitchW) / stripes, y0, pitchW / stripes + 1, pitchH);
    }
    if (style !== 'board') {
      // The cross-cut: a faint second direction of mowing.
      const bands = 12;
      for (let i = 0; i < bands; i++) {
        g.fillStyle = i % 2 ? 'rgba(255,255,255,0.025)' : 'rgba(0,0,0,0.025)';
        g.fillRect(x0, y0 + (i * pitchH) / bands, pitchW, pitchH / bands + 1);
      }
    }

    const img = g.getImageData(0, 0, W, H);
    const rand = rng(7);
    const amount = palette.noise * 255;
    for (let i = 0; i < img.data.length; i += 4) {
      const n = (rand() - 0.5) * amount;
      img.data[i] += n * 0.6;
      img.data[i + 1] += n;
      img.data[i + 2] += n * 0.5;
    }
    g.putImageData(img, 0, 0);

    // A soft vignette towards the corners, so the edge of the world is not a
    // hard line where the texture ends.
    const vignette = g.createRadialGradient(W / 2, H / 2, H * 0.45, W / 2, H / 2, W * 0.62);
    vignette.addColorStop(0, 'rgba(0,0,0,0)');
    vignette.addColorStop(1, 'rgba(0,0,0,0.28)');
    g.fillStyle = vignette;
    g.fillRect(0, 0, W, H);

    const t = texture(c);
    t.generateMipmaps = true;
    return t;
  });
}

/**
 * A shirt, unwrapped. `u` runs around the body and `v` from feet to head, so
 * vertical stripes are bands in `u` and hoops are bands in `v`.
 */
export function kitTexture(primary: string, secondary: string, pattern: KitPattern): THREE.Texture {
  return cached(`kit:${primary}:${secondary}:${pattern}`, () => {
    const S = 256;
    const [c, g] = canvas(S, S);
    g.fillStyle = primary;
    g.fillRect(0, 0, S, S);
    g.fillStyle = secondary;

    switch (pattern) {
      case 'stripes':
        for (let i = 0; i < 10; i += 2) g.fillRect((i * S) / 10, 0, S / 10, S);
        break;
      case 'hoops':
        for (let i = 1; i < 9; i += 2) g.fillRect(0, (i * S) / 9, S, S / 9);
        break;
      case 'halves':
        g.fillRect(0, 0, S / 2, S);
        break;
      case 'sash':
        // Drawn twice, offset by the texture width, so the band is continuous
        // where the wrap meets.
        for (const dx of [0, -S, S]) {
          g.beginPath();
          g.moveTo(dx + S * 0.05, S);
          g.lineTo(dx + S * 0.3, S);
          g.lineTo(dx + S * 0.95, 0);
          g.lineTo(dx + S * 0.7, 0);
          g.closePath();
          g.fill();
        }
        break;
    }

    // Shorts: the bottom of the body in the secondary colour, which is what
    // makes a figure read as a footballer rather than a peg.
    if (pattern !== 'halves') {
      g.fillStyle = secondary;
      g.fillRect(0, S * 0.8, S, S * 0.2);
    }
    return texture(c);
  });
}

/** A shirt number or a name, with an outline so it reads on any grass. */
export function labelTexture(text: string, fill: string, kind: 'number' | 'name'): THREE.Texture {
  return cached(`label:${kind}:${fill}:${text}`, () => {
    const W = kind === 'number' ? 128 : 512;
    const H = 128;
    const [c, g] = canvas(W, H);
    g.textAlign = 'center';
    g.textBaseline = 'middle';
    g.font =
      kind === 'number'
        ? `800 84px system-ui, -apple-system, 'Segoe UI', sans-serif`
        : `700 64px system-ui, -apple-system, 'Segoe UI', sans-serif`;
    g.lineJoin = 'round';
    g.lineWidth = kind === 'number' ? 14 : 12;
    g.strokeStyle = 'rgba(10,14,18,0.85)';
    g.strokeText(text, W / 2, H / 2 + 4);
    g.fillStyle = fill;
    g.fillText(text, W / 2, H / 2 + 4);
    return texture(c);
  });
}

/**
 * The ball: white with black patches at the twelve vertices of an
 * icosahedron, which is where a classic ball's pentagons sit. Painted on an
 * equirectangular map, so each patch is widened by 1/cos(latitude) to stay
 * round once wrapped onto the sphere.
 */
export function ballTexture(): THREE.Texture {
  return cached('ball', () => {
    const W = 512;
    const H = 256;
    const [c, g] = canvas(W, H);
    g.fillStyle = '#f8fafc';
    g.fillRect(0, 0, W, H);

    const phi = (1 + Math.sqrt(5)) / 2;
    const verts: [number, number, number][] = [
      [-1, phi, 0],
      [1, phi, 0],
      [-1, -phi, 0],
      [1, -phi, 0],
      [0, -1, phi],
      [0, 1, phi],
      [0, -1, -phi],
      [0, 1, -phi],
      [phi, 0, -1],
      [phi, 0, 1],
      [-phi, 0, -1],
      [-phi, 0, 1],
    ];
    g.fillStyle = '#111827';
    for (const [x, y, z] of verts) {
      const len = Math.hypot(x, y, z);
      const lat = Math.asin(y / len);
      const lon = Math.atan2(z, x);
      const u = ((lon / (Math.PI * 2) + 0.5) * W) % W;
      const v = (0.5 - lat / Math.PI) * H;
      const r = H * 0.09;
      const rx = Math.min(W / 2, r / Math.max(0.2, Math.cos(lat)));
      for (const dx of [0, -W, W]) {
        g.beginPath();
        g.ellipse(u + dx, v, rx, r, 0, 0, Math.PI * 2);
        g.fill();
      }
    }
    return texture(c);
  });
}

/** A repeating net mesh, transparent between the cords. */
export function netTexture(): THREE.Texture {
  return cached('net', () => {
    const S = 64;
    const [c, g] = canvas(S, S);
    g.strokeStyle = 'rgba(255,255,255,0.85)';
    g.lineWidth = 3;
    g.beginPath();
    g.moveTo(0, S / 2);
    g.lineTo(S, S / 2);
    g.moveTo(S / 2, 0);
    g.lineTo(S / 2, S);
    g.stroke();
    const t = texture(c);
    t.wrapS = THREE.RepeatWrapping;
    t.wrapT = THREE.RepeatWrapping;
    return t;
  });
}

/** Text laid flat on the grass, for labels drawn on the board. */
export function groundTextTexture(
  text: string,
  color: string,
): { tex: THREE.Texture; aspect: number } {
  const font = `700 96px system-ui, -apple-system, 'Segoe UI', sans-serif`;
  const [, pg] = canvas(8, 8);
  pg.font = font;
  const width = Math.max(64, Math.ceil(pg.measureText(text).width) + 48);
  const tex = cached(`ground-text:${color}:${text}`, () => {
    const [c, g] = canvas(width, 140);
    g.font = font;
    g.textAlign = 'center';
    g.textBaseline = 'middle';
    g.lineJoin = 'round';
    g.lineWidth = 16;
    g.strokeStyle = 'rgba(10,14,18,0.75)';
    g.strokeText(text, width / 2, 74);
    g.fillStyle = color;
    g.fillText(text, width / 2, 74);
    return texture(c);
  });
  return { tex, aspect: width / 140 };
}

/** A soft round glow, for the spotlight pool and selection halo. */
export function glowTexture(): THREE.Texture {
  return cached('glow', () => {
    const S = 128;
    const [c, g] = canvas(S, S);
    const grad = g.createRadialGradient(S / 2, S / 2, 0, S / 2, S / 2, S / 2);
    grad.addColorStop(0, 'rgba(255,255,255,1)');
    grad.addColorStop(0.5, 'rgba(255,255,255,0.45)');
    grad.addColorStop(1, 'rgba(255,255,255,0)');
    g.fillStyle = grad;
    g.fillRect(0, 0, S, S);
    return texture(c);
  });
}
