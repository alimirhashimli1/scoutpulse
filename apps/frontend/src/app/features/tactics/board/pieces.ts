import * as THREE from 'three';

import { BoardObject, EQUIPMENT_COLORS, EquipmentKind, TacticDoc, playerLook } from './model';
import { ballTexture, kitTexture, labelTexture, netTexture } from './textures';

/**
 * Meshes for everything that stands on the board.
 *
 * Sizes are exaggerated on purpose. A real player seen from above a 105-metre
 * pitch is a speck; tactics boards draw figures several times life size so a
 * shape can be read at a glance. Equipment is scaled up less, so a cone still
 * looks small next to a player.
 *
 * Every piece is a Group tagged with its object id, which is how a click on
 * any of its parts is traced back to the thing on the board.
 */

const materials = new Map<string, THREE.Material>();

function standard(color: string, opts: { rough?: number; metal?: number; emissive?: string } = {}) {
  const key = `std:${color}:${opts.rough ?? 0.55}:${opts.metal ?? 0}:${opts.emissive ?? ''}`;
  let m = materials.get(key);
  if (!m) {
    m = new THREE.MeshStandardMaterial({
      color,
      roughness: opts.rough ?? 0.55,
      metalness: opts.metal ?? 0,
      emissive: opts.emissive ?? '#000000',
      emissiveIntensity: opts.emissive ? 0.35 : 0,
    });
    materials.set(key, m);
  }
  return m;
}

export function disposeMaterials(): void {
  materials.forEach((m) => m.dispose());
  materials.clear();
}

function mesh(
  geometry: THREE.BufferGeometry,
  material: THREE.Material,
  shadows = true,
): THREE.Mesh {
  const m = new THREE.Mesh(geometry, material);
  m.castShadow = shadows;
  m.receiveShadow = shadows;
  return m;
}

/** A key that changes whenever the piece would look different. */
export function appearanceKey(doc: TacticDoc, obj: BoardObject): string {
  if (obj.kind === 'player') {
    const look = playerLook(doc, obj);
    return [
      'player',
      look.primary,
      look.secondary,
      look.pattern,
      look.numbers,
      obj.number ?? '',
      obj.name ?? '',
      obj.bib ?? '',
      obj.size ?? 1,
      obj.keeper ? 'gk' : '',
    ].join('|');
  }
  return [obj.kind, obj.color ?? '', obj.size ?? 1].join('|');
}

export function buildPiece(doc: TacticDoc, obj: BoardObject): THREE.Group {
  const group =
    obj.kind === 'player'
      ? player(doc, obj)
      : equipment(obj.kind, obj.color ?? EQUIPMENT_COLORS[obj.kind]);
  group.scale.setScalar(baseScale(obj.kind) * (obj.size ?? 1));
  group.userData['objectId'] = obj.id;
  group.traverse((child) => (child.userData['objectId'] = obj.id));
  return group;
}

/**
 * How much bigger than its modelled size each kind is drawn. Figures are
 * modelled roughly to scale and enlarged here, in one place, so the balance
 * between a player, a ball and a cone can be tuned without touching meshes.
 * The ball gets the most: life size it would vanish on a full pitch.
 */
export function baseScale(kind: BoardObject['kind']): number {
  switch (kind) {
    case 'player':
      return 1.3;
    case 'ball':
      return 2.2;
    case 'goal':
      return 1;
    default:
      return 1.25;
  }
}

// --- players -------------------------------------------------------------

/** Heights the engine needs: where the label floats, how big the halo is. */
export const PLAYER_HEIGHT = 2.9;
export const PLAYER_RADIUS = 0.95;

function player(doc: TacticDoc, obj: BoardObject): THREE.Group {
  const look = playerLook(doc, obj);
  const g = new THREE.Group();

  // The base: a disc in the secondary colour, which is what a top-down view
  // mostly sees, so two teams in similar shirts can still be told apart.
  const base = mesh(
    new THREE.CylinderGeometry(PLAYER_RADIUS, PLAYER_RADIUS * 1.04, 0.14, 40),
    standard(look.secondary, { rough: 0.4 }),
  );
  base.position.y = 0.07;
  g.add(base);
  const rim = mesh(
    new THREE.TorusGeometry(PLAYER_RADIUS, 0.07, 10, 48),
    standard(look.primary, { rough: 0.35 }),
    false,
  );
  rim.rotation.x = Math.PI / 2;
  rim.position.y = 0.14;
  g.add(rim);

  const shirt = new THREE.MeshStandardMaterial({
    map: kitTexture(look.primary, look.secondary, look.pattern),
    roughness: 0.62,
  });
  const body = mesh(new THREE.CapsuleGeometry(0.6, 0.95, 8, 24), shirt);
  body.position.y = 0.16 + 0.6 + 0.475;
  g.add(body);

  if (obj.bib) {
    const bib = mesh(
      new THREE.CylinderGeometry(0.635, 0.635, 0.7, 28, 1, true),
      new THREE.MeshStandardMaterial({ color: obj.bib, roughness: 0.8, side: THREE.DoubleSide }),
    );
    bib.position.y = 1.55;
    g.add(bib);
  }

  const head = mesh(new THREE.SphereGeometry(0.4, 24, 16), standard('#e2e8f0', { rough: 0.45 }));
  head.position.y = 2.42;
  g.add(head);

  if (obj.number) {
    const label = new THREE.Sprite(
      new THREE.SpriteMaterial({
        map: labelTexture(
          obj.number,
          look.numbers === look.primary ? '#ffffff' : look.numbers,
          'number',
        ),
        depthTest: false,
      }),
    );
    label.scale.set(2.5, 2.5, 1);
    label.position.y = PLAYER_HEIGHT + 0.95;
    label.renderOrder = 10;
    g.add(label);
  }
  if (obj.name) {
    const name = new THREE.Sprite(
      new THREE.SpriteMaterial({
        map: labelTexture(obj.name, '#ffffff', 'name'),
        depthTest: false,
      }),
    );
    name.scale.set(4.4, 1.1, 1);
    name.position.y = obj.number ? PLAYER_HEIGHT + 2.3 : PLAYER_HEIGHT + 0.95;
    name.renderOrder = 10;
    g.add(name);
  }
  return g;
}

// --- equipment -----------------------------------------------------------

function equipment(kind: EquipmentKind, color: string): THREE.Group {
  const g = new THREE.Group();
  const mat = standard(color);

  switch (kind) {
    case 'ball': {
      const ball = mesh(
        new THREE.SphereGeometry(0.42, 32, 20),
        new THREE.MeshStandardMaterial({ map: ballTexture(), roughness: 0.35, color }),
      );
      ball.position.y = 0.42;
      ball.name = 'ball';
      g.add(ball);
      break;
    }
    case 'cone': {
      const cone = mesh(new THREE.ConeGeometry(0.3, 0.85, 28), standard(color, { rough: 0.4 }));
      cone.position.y = 0.47;
      const plate = mesh(new THREE.BoxGeometry(0.62, 0.05, 0.62), standard(color, { rough: 0.5 }));
      plate.position.y = 0.025;
      const band = mesh(
        new THREE.CylinderGeometry(0.13, 0.17, 0.14, 24),
        standard('#ffffff', { rough: 0.4 }),
        false,
      );
      band.position.y = 0.6;
      g.add(cone, plate, band);
      break;
    }
    case 'disc': {
      const disc = mesh(
        new THREE.CylinderGeometry(0.1, 0.4, 0.16, 32),
        standard(color, { rough: 0.45 }),
      );
      disc.position.y = 0.08;
      g.add(disc);
      break;
    }
    case 'pole': {
      const pole = mesh(
        new THREE.CylinderGeometry(0.05, 0.05, 1.9, 16),
        standard(color, { rough: 0.35 }),
      );
      pole.position.y = 1.0;
      const base = mesh(new THREE.CylinderGeometry(0.2, 0.26, 0.08, 24), standard('#1f2937'));
      base.position.y = 0.04;
      g.add(pole, base);
      break;
    }
    case 'mannequin': {
      const body = mesh(new THREE.CapsuleGeometry(0.34, 1.0, 6, 20), mat);
      body.scale.z = 0.3;
      body.position.y = 1.0;
      const head = mesh(new THREE.SphereGeometry(0.2, 20, 14), mat);
      head.position.y = 2.0;
      const spike = mesh(new THREE.CylinderGeometry(0.03, 0.03, 0.4, 8), standard('#475569'));
      spike.position.y = 0.2;
      g.add(body, head, spike);
      break;
    }
    case 'hurdle': {
      for (const z of [-0.5, 0.5]) {
        const leg = mesh(new THREE.BoxGeometry(0.06, 0.55, 0.06), mat);
        leg.position.set(0, 0.275, z);
        const foot = mesh(new THREE.BoxGeometry(0.4, 0.04, 0.06), mat);
        foot.position.set(0, 0.02, z);
        g.add(leg, foot);
      }
      const bar = mesh(new THREE.BoxGeometry(0.07, 0.07, 1.1), mat);
      bar.position.y = 0.55;
      g.add(bar);
      break;
    }
    case 'ladder': {
      const rungs = 9;
      const len = 4.4;
      for (const z of [-0.3, 0.3]) {
        const rail = mesh(new THREE.BoxGeometry(len, 0.03, 0.05), mat, false);
        rail.position.set(0, 0.03, z);
        rail.receiveShadow = true;
        g.add(rail);
      }
      for (let i = 0; i < rungs; i++) {
        const rung = mesh(new THREE.BoxGeometry(0.05, 0.03, 0.62), mat, false);
        rung.position.set(-len / 2 + (i * len) / (rungs - 1), 0.035, 0);
        g.add(rung);
      }
      break;
    }
    case 'ring': {
      const ring = mesh(
        new THREE.TorusGeometry(0.5, 0.045, 10, 48),
        standard(color, { rough: 0.35 }),
      );
      ring.rotation.x = Math.PI / 2;
      ring.position.y = 0.045;
      g.add(ring);
      break;
    }
    case 'minigoal':
      g.add(goal(2.4, 1.2, 1.0, color, 0.05));
      break;
    case 'goal':
      g.add(goal(7.32, 2.44, 2.0, color, 0.08));
      break;
    case 'popup':
      g.add(popup(color));
      break;
    case 'flag': {
      const pole = mesh(
        new THREE.CylinderGeometry(0.03, 0.03, 1.6, 10),
        standard('#f8fafc', { rough: 0.3 }),
      );
      pole.position.y = 0.8;
      const shape = new THREE.Shape();
      shape.moveTo(0, 0);
      shape.lineTo(0.7, -0.22);
      shape.lineTo(0, -0.44);
      shape.closePath();
      const cloth = mesh(
        new THREE.ShapeGeometry(shape),
        new THREE.MeshStandardMaterial({ color, side: THREE.DoubleSide, roughness: 0.7 }),
      );
      cloth.position.set(0.03, 1.58, 0);
      g.add(pole, cloth);
      break;
    }
  }
  return g;
}

/**
 * A goal whose mouth is the plane x = 0, net extending back towards −x.
 * Rotation then points it anywhere; the pitch's own goals are two of these.
 */
export function goal(
  width: number,
  height: number,
  depth: number,
  color: string,
  post: number,
): THREE.Group {
  const g = new THREE.Group();
  const frame = standard(color, { rough: 0.3, metal: 0.1 });
  const hw = width / 2;

  const upright = new THREE.CylinderGeometry(post, post, height, 16);
  for (const z of [-hw, hw]) {
    const p = mesh(upright, frame);
    p.position.set(0, height / 2, z);
    g.add(p);
  }
  const bar = mesh(new THREE.CylinderGeometry(post, post, width + post * 2, 16), frame);
  bar.rotation.x = Math.PI / 2;
  bar.position.set(0, height, 0);
  g.add(bar);

  // Back frame, thinner, along the ground and up the back corners.
  const thin = standard('#cbd5e1', { rough: 0.5 });
  const backBar = mesh(new THREE.CylinderGeometry(post * 0.5, post * 0.5, width, 8), thin);
  backBar.rotation.x = Math.PI / 2;
  backBar.position.set(-depth, post * 0.5, 0);
  g.add(backBar);
  const backHeight = height * 0.75;
  for (const z of [-hw, hw]) {
    const stay = mesh(new THREE.CylinderGeometry(post * 0.5, post * 0.5, backHeight, 8), thin);
    stay.position.set(-depth, backHeight / 2, z);
    g.add(stay);
  }

  const net = new THREE.MeshStandardMaterial({
    map: netTexture(),
    transparent: true,
    side: THREE.DoubleSide,
    depthWrite: false,
    roughness: 0.9,
  });
  const cell = 0.16;

  // The roof slopes from the crossbar down to the back stanchions.
  const roofLen = Math.hypot(depth, height - backHeight);
  g.add(
    mesh(
      quadGeometry(
        [
          [0, height, -hw],
          [0, height, hw],
          [-depth, backHeight, hw],
          [-depth, backHeight, -hw],
        ],
        width / cell,
        roofLen / cell,
      ),
      net,
      false,
    ),
  );
  g.add(
    mesh(
      quadGeometry(
        [
          [-depth, 0, -hw],
          [-depth, 0, hw],
          [-depth, backHeight, hw],
          [-depth, backHeight, -hw],
        ],
        width / cell,
        backHeight / cell,
      ),
      net,
      false,
    ),
  );

  for (const z of [-hw, hw]) {
    const sideShape = new THREE.Shape();
    sideShape.moveTo(0, 0);
    sideShape.lineTo(0, height);
    sideShape.lineTo(-depth, backHeight);
    sideShape.lineTo(-depth, 0);
    sideShape.closePath();
    const side = new THREE.ShapeGeometry(sideShape);
    scaleUv(side, 1 / cell);
    const s = mesh(side, net, false);
    s.position.z = z;
    g.add(s);
  }
  return g;
}

/**
 * Two triangles through four corners, in order, with the texture repeated
 * `ru` × `rv` times. Simpler than placing and rotating a plane to fit a sloped
 * roof, and exact by construction.
 */
function quadGeometry(
  corners: [number, number, number][],
  ru: number,
  rv: number,
): THREE.BufferGeometry {
  const geo = new THREE.BufferGeometry();
  geo.setAttribute('position', new THREE.Float32BufferAttribute(corners.flat(), 3));
  geo.setAttribute('uv', new THREE.Float32BufferAttribute([0, 0, ru, 0, ru, rv, 0, rv], 2));
  geo.setIndex([0, 1, 2, 0, 2, 3]);
  geo.computeVertexNormals();
  return geo;
}

function popup(color: string): THREE.Group {
  const g = new THREE.Group();
  const w = 1.6;
  const h = 1.0;
  const arch = new THREE.EllipseCurve(0, 0, w / 2, h, 0, Math.PI, false);
  const pts = arch.getPoints(24).map((p) => new THREE.Vector3(0, p.y, p.x));
  const frame = mesh(
    new THREE.TubeGeometry(new THREE.CatmullRomCurve3(pts), 32, 0.035, 8, false),
    standard(color, { rough: 0.4 }),
  );
  g.add(frame);
  const back = mesh(
    new THREE.TubeGeometry(
      new THREE.CatmullRomCurve3(pts.map((p) => new THREE.Vector3(-0.9, p.y * 0.8, p.z))),
      32,
      0.03,
      8,
      false,
    ),
    standard(color, { rough: 0.4 }),
  );
  g.add(back);
  const shape = new THREE.Shape();
  arch.getPoints(24).forEach((p, i) => (i ? shape.lineTo(p.x, p.y) : shape.moveTo(p.x, p.y)));
  const netGeo = new THREE.ShapeGeometry(shape);
  scaleUv(netGeo, 1 / 0.14);
  const net = mesh(
    netGeo,
    new THREE.MeshStandardMaterial({
      map: netTexture(),
      transparent: true,
      side: THREE.DoubleSide,
      depthWrite: false,
    }),
    false,
  );
  net.rotation.y = Math.PI / 2;
  net.position.x = -0.9;
  net.scale.y = 0.8;
  g.add(net);
  return g;
}

/**
 * Repeats the net texture by metres rather than once per shape, so a goal and
 * a mini goal have cords the same distance apart. A ShapeGeometry's UVs are
 * its coordinates in metres, so scaling them by 1/cell is exactly that.
 */
function scaleUv(geo: THREE.BufferGeometry, s: number): void {
  const uv = geo.getAttribute('uv') as THREE.BufferAttribute;
  for (let i = 0; i < uv.count; i++) uv.setXY(i, uv.getX(i) * s, uv.getY(i) * s);
  uv.needsUpdate = true;
}

/** Frees a piece's per-instance GPU resources. Shared materials are kept. */
export function disposePiece(group: THREE.Object3D): void {
  group.traverse((child) => {
    if (child instanceof THREE.Mesh) {
      child.geometry.dispose();
      const mat = child.material as THREE.Material;
      if (![...materials.values()].includes(mat)) mat.dispose();
    } else if (child instanceof THREE.Sprite) {
      child.material.dispose();
    }
  });
}
