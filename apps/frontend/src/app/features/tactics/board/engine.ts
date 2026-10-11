import * as THREE from 'three';
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js';

import {
  curveMid,
  dashes,
  distToPolyline,
  easeInOut,
  insidePolygon,
  sampleCurve,
} from './geometry';
import {
  BoardObject,
  Drawing,
  DrawingKind,
  HALF_LENGTH,
  HALF_WIDTH,
  PitchSettings,
  RUNOFF,
  TacticDoc,
  Vec2,
  clampToArea,
  movedInto,
  positionAt,
} from './model';
import { Frame } from './playback';
import {
  PLAYER_RADIUS,
  appearanceKey,
  baseScale,
  buildPiece,
  disposeMaterials,
  disposePiece,
  goal,
} from './pieces';
import {
  DEFAULT_STROKE,
  buildDrawing,
  buildMarkings,
  disposeGroup,
  drawingPath,
  ribbon,
  setOpacity,
} from './strokes';
import { disposeTextures, glowTexture, grassTexture } from './textures';

/**
 * The 3D board.
 *
 * A plain class with no Angular in it: it owns a WebGL canvas, draws whatever
 * frame it is handed, and reports what the pointer did in pitch coordinates.
 * Deciding what a gesture *means* — adding a player, moving one in this step,
 * finishing a zone — is the editor's job, so the same engine serves the
 * read-only viewer unchanged.
 *
 * Board (x, y) maps to world (x, 0, y): the pitch lies in the XZ plane, and
 * a top-down camera sees +y as "down the screen", as on paper.
 */

export type CameraView = 'top' | 'tactical' | 'broadcast' | 'behind';

/** What a click landed on. */
export type Target = { type: 'object'; id: string } | { type: 'drawing'; id: string };

/** Which tool the pointer is acting as. */
export type Tool = 'select' | 'place' | DrawingKind;

export type DrawingChange = { index: number; at: Vec2 } | { control: Vec2 } | { offset: Vec2 };

export interface EngineHandlers {
  select?(target: Target | null): void;
  moveObject?(id: string, at: Vec2, done: boolean): void;
  bend?(id: string, mid: Vec2, done: boolean): void;
  editDrawing?(id: string, change: DrawingChange, done: boolean): void;
  place?(at: Vec2): void;
  /** A stroke being drawn with a drawing tool; `done` on release. */
  stroke?(points: Vec2[], done: boolean): void;
}

export interface EditState {
  /**
   * This board is in the editor. Decides the mouse layout — left edits, right
   * turns the camera — and stays true even while editing is paused, so the
   * left button never silently becomes a camera control mid-session.
   */
  editor: boolean;
  /** Edits are accepted right now: false while playing or between steps. */
  editing: boolean;
  step: number;
  selection: Target | null;
  /** Paths of everything that moved into this step, with bend handles. */
  trails: boolean;
}

type Handle =
  | { kind: 'bend'; id: string }
  | { kind: 'point'; id: string; index: number }
  | { kind: 'control'; id: string };

type Gesture =
  | { kind: 'object'; id: string; offset: Vec2; moved: boolean }
  | { kind: 'handle'; handle: Handle }
  | { kind: 'drawing'; id: string; last: Vec2; moved: boolean }
  | { kind: 'stroke'; points: Vec2[] };

const ACCENT = '#38bdf8';

export class BoardEngine {
  readonly canvas: HTMLCanvasElement;
  handlers: EngineHandlers = {};
  tool: Tool = 'select';

  private readonly renderer: THREE.WebGLRenderer;
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.PerspectiveCamera(38, 1, 0.5, 900);
  private readonly controls: OrbitControls;
  private readonly raycaster = new THREE.Raycaster();
  private readonly ground = new THREE.Plane(new THREE.Vector3(0, 1, 0), 0);
  private readonly resize: ResizeObserver;
  private readonly clock = new THREE.Clock();
  private frameId = 0;
  private disposed = false;

  private readonly world = new THREE.Group();
  private readonly piecesRoot = new THREE.Group();
  private readonly drawingsRoot = new THREE.Group();
  private readonly overlayRoot = new THREE.Group();
  private readonly spotsRoot = new THREE.Group();
  private readonly previewRoot = new THREE.Group();

  private readonly sun = new THREE.DirectionalLight('#fff7e8', 2.4);
  private readonly hemi = new THREE.HemisphereLight('#dbeafe', '#264d2a', 0.9);

  private doc: TacticDoc | null = null;
  private pitchKey = '';
  private readonly pieces = new Map<string, { key: string; group: THREE.Group; last?: Vec2 }>();
  private drawings: Drawing[] = [];
  private drawingsKey: Drawing[] | null = null;
  private drawingOpacity = 1;
  private edit: EditState = {
    editor: false,
    editing: false,
    step: 0,
    selection: null,
    trails: false,
  };
  private frame: Frame | null = null;
  private gesture: Gesture | null = null;
  private polygon: Vec2[] = [];
  private selectionRing: THREE.Mesh;
  private tween: {
    from: [THREE.Vector3, THREE.Vector3];
    to: [THREE.Vector3, THREE.Vector3];
    t: number;
  } | null = null;
  private view: CameraView = 'tactical';

  /** Called every animation frame with seconds elapsed; playback hangs off it. */
  onTick: ((dt: number) => void) | null = null;

  constructor(private readonly host: HTMLElement) {
    this.renderer = new THREE.WebGLRenderer({
      antialias: true,
      powerPreference: 'high-performance',
    });
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    this.renderer.shadowMap.enabled = true;
    this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping;
    this.renderer.toneMappingExposure = 1.05;
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.canvas = this.renderer.domElement;
    this.canvas.style.display = 'block';
    this.canvas.style.width = '100%';
    this.canvas.style.height = '100%';
    this.canvas.style.touchAction = 'none';
    host.appendChild(this.canvas);

    this.controls = new OrbitControls(this.camera, this.canvas);
    this.controls.enableDamping = true;
    this.controls.dampingFactor = 0.09;
    this.controls.maxPolarAngle = Math.PI / 2 - 0.06;
    this.controls.minDistance = 8;
    this.controls.maxDistance = 220;
    this.controls.addEventListener('start', () => (this.tween = null));

    this.sun.position.set(-35, 90, 45);
    this.sun.castShadow = true;
    this.sun.shadow.mapSize.set(2048, 2048);
    const sc = this.sun.shadow.camera;
    sc.left = -75;
    sc.right = 75;
    sc.top = 55;
    sc.bottom = -55;
    sc.near = 10;
    sc.far = 220;
    this.sun.shadow.bias = -0.0004;
    this.sun.shadow.normalBias = 0.03;
    this.scene.add(
      this.sun,
      this.hemi,
      this.world,
      this.drawingsRoot,
      this.spotsRoot,
      this.piecesRoot,
      this.overlayRoot,
      this.previewRoot,
    );

    this.selectionRing = new THREE.Mesh(
      new THREE.RingGeometry(1.15, 1.45, 48),
      new THREE.MeshBasicMaterial({
        color: ACCENT,
        transparent: true,
        depthWrite: false,
        side: THREE.DoubleSide,
      }),
    );
    this.selectionRing.rotation.x = -Math.PI / 2;
    this.selectionRing.visible = false;
    this.scene.add(this.selectionRing);

    this.applyInteractivity();
    this.canvas.addEventListener('pointerdown', this.onPointerDown);
    this.canvas.addEventListener('pointermove', this.onPointerMove);
    this.canvas.addEventListener('pointerup', this.onPointerUp);
    this.canvas.addEventListener('pointercancel', this.onPointerUp);
    this.canvas.addEventListener('dblclick', this.onDoubleClick);
    this.canvas.addEventListener('contextmenu', (e) => e.preventDefault());

    this.resize = new ResizeObserver(() => this.fit());
    this.resize.observe(host);
    this.fit();
    this.setView('tactical', true);
    this.loop();
  }

  // --- inputs -------------------------------------------------------------

  setDoc(doc: TacticDoc): void {
    this.doc = doc;
    const key = JSON.stringify(doc.pitch);
    if (key !== this.pitchKey) {
      this.pitchKey = key;
      this.buildWorld(doc.pitch);
      this.setView(this.view, true);
    }

    const alive = new Set<string>();
    for (const obj of doc.objects) {
      alive.add(obj.id);
      const k = appearanceKey(doc, obj);
      const existing = this.pieces.get(obj.id);
      if (existing && existing.key === k) {
        existing.group.rotation.y = -THREE.MathUtils.degToRad(obj.rotation ?? 0);
        continue;
      }
      if (existing) {
        this.piecesRoot.remove(existing.group);
        disposePiece(existing.group);
      }
      const group = buildPiece(doc, obj);
      group.rotation.y = -THREE.MathUtils.degToRad(obj.rotation ?? 0);
      group.visible = false;
      this.piecesRoot.add(group);
      this.pieces.set(obj.id, { key: k, group, last: existing?.last });
    }
    for (const [id, piece] of this.pieces) {
      if (!alive.has(id)) {
        this.piecesRoot.remove(piece.group);
        disposePiece(piece.group);
        this.pieces.delete(id);
      }
    }
    this.refreshOverlays();
  }

  /** Draws a moment: where everything stands, and what is on the grass. */
  setFrame(frame: Frame): void {
    this.frame = frame;
    for (const [id, piece] of this.pieces) {
      const at = frame.placements.get(id);
      const g = piece.group;
      if (!at) {
        g.visible = false;
        continue;
      }
      g.visible = true;
      g.position.set(at.x, at.h, at.y);
      const obj = this.doc?.objects.find((o) => o.id === id);
      const size = obj?.size ?? 1;
      g.scale.setScalar(baseScale(obj?.kind ?? 'cone') * size * Math.max(0.001, at.appear));
      if (obj?.kind === 'ball') this.spin(piece, g, [at.x, at.y]);
      piece.last = [at.x, at.y];
    }

    if (frame.drawings !== this.drawingsKey) {
      this.drawingsKey = frame.drawings;
      this.drawings = frame.drawings;
      this.drawingOpacity = frame.drawingOpacity;
      this.rebuildDrawings();
    } else if (Math.abs(frame.drawingOpacity - this.drawingOpacity) > 0.001) {
      // During playback only the fade changes; rebuilding sixty times a second
      // would allocate geometry for nothing.
      this.drawingOpacity = frame.drawingOpacity;
      this.drawingsRoot.children.forEach((g) => setOpacity(g, frame.drawingOpacity));
    }
    this.rebuildSpots(frame.highlights);
    this.refreshOverlays();
  }

  setEditState(state: EditState): void {
    this.edit = state;
    this.applyInteractivity();
    this.refreshOverlays();
  }

  /** A drawing in progress, shown before it is committed. */
  setPreview(drawing: Drawing | null): void {
    this.clear(this.previewRoot);
    if (drawing) this.previewRoot.add(buildDrawing(drawing, 0.85, 50));
  }

  /** Abandons an in-progress polygon or stroke. */
  cancelGesture(): void {
    this.gesture = null;
    this.polygon = [];
    this.setPreview(null);
  }

  setView(view: CameraView, instant = false): void {
    this.view = view;
    const [pos, target] = this.framing(view);
    if (instant) {
      this.camera.position.copy(pos);
      this.controls.target.copy(target);
      this.tween = null;
      this.controls.update();
      return;
    }
    this.tween = {
      from: [this.camera.position.clone(), this.controls.target.clone()],
      to: [pos, target],
      t: 0,
    };
  }

  get currentView(): CameraView {
    return this.view;
  }

  dispose(): void {
    this.disposed = true;
    cancelAnimationFrame(this.frameId);
    this.resize.disconnect();
    this.controls.dispose();
    this.canvas.removeEventListener('pointerdown', this.onPointerDown);
    this.canvas.removeEventListener('pointermove', this.onPointerMove);
    this.canvas.removeEventListener('pointerup', this.onPointerUp);
    this.canvas.removeEventListener('pointercancel', this.onPointerUp);
    this.canvas.removeEventListener('dblclick', this.onDoubleClick);
    for (const p of this.pieces.values()) disposePiece(p.group);
    this.pieces.clear();
    for (const root of [
      this.world,
      this.drawingsRoot,
      this.overlayRoot,
      this.spotsRoot,
      this.previewRoot,
    ])
      this.clear(root);
    disposeMaterials();
    disposeTextures();
    this.renderer.dispose();
    this.canvas.remove();
  }

  // --- the world ----------------------------------------------------------

  private buildWorld(pitch: PitchSettings): void {
    this.clear(this.world);
    const night = pitch.style === 'night';
    const board = pitch.style === 'board';

    this.scene.background = skyTexture(pitch.style);
    this.scene.fog = board ? null : new THREE.Fog(night ? '#070b16' : '#c9d8e6', 160, 420);

    this.hemi.color.set(night ? '#8fb3ff' : '#e0f2fe');
    this.hemi.groundColor.set(night ? '#0b1a10' : '#2c4a2a');
    this.hemi.intensity = night ? 0.55 : board ? 1.6 : 0.95;
    this.sun.color.set(night ? '#e8f0ff' : '#fff4e0');
    this.sun.intensity = night ? 2.0 : board ? 1.2 : 2.5;
    this.sun.position.set(night ? 0 : -35, 90, night ? 10 : 45);

    const totalL = (HALF_LENGTH + RUNOFF) * 2;
    const totalW = (HALF_WIDTH + RUNOFF) * 2;
    const grass = new THREE.Mesh(
      new THREE.PlaneGeometry(totalL, totalW),
      new THREE.MeshStandardMaterial({ map: grassTexture(pitch.style), roughness: 0.95 }),
    );
    grass.rotation.x = -Math.PI / 2;
    grass.receiveShadow = true;
    this.world.add(grass);

    const apron = new THREE.Mesh(
      new THREE.PlaneGeometry(600, 600),
      new THREE.MeshStandardMaterial({
        color: board ? '#14231c' : night ? '#10161c' : '#5b6670',
        roughness: 1,
      }),
    );
    apron.rotation.x = -Math.PI / 2;
    apron.position.y = -0.05;
    apron.receiveShadow = true;
    this.world.add(apron);

    const lineColor = board ? '#f1f5f9' : '#ffffff';
    this.world.add(buildMarkings(pitch.markings, lineColor, board ? 0.75 : 0.92));

    if (pitch.markings === 'standard') {
      for (const side of [-1, 1]) {
        const g = goal(7.32, 2.44, 2.0, '#f8fafc', 0.08);
        g.position.x = side * HALF_LENGTH;
        // Mouths face the centre spot.
        g.rotation.y = side < 0 ? Math.PI : 0;
        g.traverse((c) => (c.userData['static'] = true));
        this.world.add(g);
      }
    }

    if (pitch.stadium && !board) this.world.add(stadium(night));
  }

  /** Camera position and target for a view of the current layout. */
  private framing(view: CameraView): [THREE.Vector3, THREE.Vector3] {
    const layout = this.doc?.pitch.layout ?? 'full';
    const bounds: Record<string, [number, number, number, number]> = {
      full: [-HALF_LENGTH - 2, HALF_LENGTH + 2, -HALF_WIDTH - 2, HALF_WIDTH + 2],
      half: [-2, HALF_LENGTH + 3, -HALF_WIDTH - 2, HALF_WIDTH + 2],
      third: [HALF_LENGTH - 37, HALF_LENGTH + 3, -HALF_WIDTH - 1, HALF_WIDTH + 1],
      box: [HALF_LENGTH - 26, HALF_LENGTH + 3, -24, 24],
      drill: [-24, 24, -17, 17],
    };
    const [x0, x1, y0, y1] = bounds[layout] ?? bounds['full'];
    const cx = (x0 + x1) / 2;
    const cy = (y0 + y1) / 2;
    const w = x1 - x0;
    const d = y1 - y0;
    const aspect = this.camera.aspect || 1.6;
    const vfov = THREE.MathUtils.degToRad(this.camera.fov);
    const hfov = 2 * Math.atan(Math.tan(vfov / 2) * aspect);
    const fitTop = Math.max(d / 2 / Math.tan(vfov / 2), w / 2 / Math.tan(hfov / 2));
    const target = new THREE.Vector3(cx, 0, cy);

    switch (view) {
      case 'top':
        // A hair off vertical, so orbiting from here has a direction to go.
        return [new THREE.Vector3(cx, fitTop * 1.04, cy + 0.01), target];
      case 'broadcast': {
        const dist = Math.max(fitTop * 0.98, 40);
        const angle = THREE.MathUtils.degToRad(24);
        return [
          new THREE.Vector3(cx, Math.sin(angle) * dist, cy + Math.cos(angle) * dist),
          target.setZ(cy - d * 0.12),
        ];
      }
      case 'behind': {
        const dist = Math.max(fitTop * 0.9, 36);
        const angle = THREE.MathUtils.degToRad(26);
        return [
          new THREE.Vector3(x0 - Math.cos(angle) * dist * 0.75, Math.sin(angle) * dist, cy),
          new THREE.Vector3(cx + w * 0.1, 0, cy),
        ];
      }
      default: {
        const dist = fitTop * 1.12;
        const angle = THREE.MathUtils.degToRad(56);
        return [new THREE.Vector3(cx, Math.sin(angle) * dist, cy + Math.cos(angle) * dist), target];
      }
    }
  }

  // --- drawings, spots, overlays -------------------------------------------

  private rebuildDrawings(): void {
    this.clear(this.drawingsRoot);
    this.drawings.forEach((d, i) => this.drawingsRoot.add(buildDrawing(d, this.drawingOpacity, i)));
  }

  private spotsKey = '';
  private rebuildSpots(ids: string[]): void {
    const key = ids.join(',');
    if (key === this.spotsKey) {
      this.spotsRoot.children.forEach((spot) => {
        const id = spot.userData['for'] as string;
        const piece = this.pieces.get(id);
        if (piece) spot.position.set(piece.group.position.x, 0, piece.group.position.z);
      });
      return;
    }
    this.spotsKey = key;
    this.clear(this.spotsRoot);
    for (const id of ids) {
      const piece = this.pieces.get(id);
      if (!piece) continue;
      const spot = new THREE.Group();
      spot.userData['for'] = id;
      const beam = new THREE.Mesh(
        new THREE.CylinderGeometry(0.25, 2.6, 16, 32, 1, true),
        new THREE.MeshBasicMaterial({
          color: '#fff6d5',
          transparent: true,
          opacity: 0.16,
          blending: THREE.AdditiveBlending,
          depthWrite: false,
          side: THREE.DoubleSide,
        }),
      );
      beam.position.y = 8;
      const pool = new THREE.Mesh(
        new THREE.PlaneGeometry(6.5, 6.5),
        new THREE.MeshBasicMaterial({
          map: glowTexture(),
          color: '#fff3c4',
          transparent: true,
          opacity: 0.85,
          depthWrite: false,
          blending: THREE.AdditiveBlending,
        }),
      );
      pool.rotation.x = -Math.PI / 2;
      pool.position.y = 0.04;
      spot.add(beam, pool);
      spot.position.set(piece.group.position.x, 0, piece.group.position.z);
      this.spotsRoot.add(spot);
    }
  }

  /** Selection ring, movement trails and drag handles. */
  private refreshOverlays(): void {
    this.clear(this.overlayRoot);
    const doc = this.doc;
    const sel = this.edit.selection;
    this.selectionRing.visible = false;
    if (!doc || !this.edit.editing) return;

    if (sel?.type === 'object') {
      const piece = this.pieces.get(sel.id);
      if (piece?.group.visible) {
        const obj = doc.objects.find((o) => o.id === sel.id);
        this.selectionRing.visible = true;
        this.selectionRing.position.set(piece.group.position.x, 0.06, piece.group.position.z);
        this.selectionRing.scale.setScalar(
          radiusOf(obj) * baseScale(obj?.kind ?? 'cone') * (obj?.size ?? 1),
        );
      }
    }

    if (this.edit.trails && this.edit.step > 0) {
      const step = doc.steps[this.edit.step];
      for (const obj of doc.objects) {
        const from = movedInto(doc, this.edit.step, obj.id);
        const to = step?.pos[obj.id];
        if (!from || !to) continue;
        const bend = step.bend?.[obj.id];
        const path = sampleCurve(from, to, bend, 32);
        const trail = new THREE.Mesh(
          ribbon(dashes(path, 1.1, 0.7), 0.28, 0.05),
          new THREE.MeshBasicMaterial({
            color: '#ffffff',
            transparent: true,
            opacity: 0.75,
            depthWrite: false,
            side: THREE.DoubleSide,
          }),
        );
        this.overlayRoot.add(trail);
        const ghost = new THREE.Mesh(
          new THREE.RingGeometry(0.75, 0.95, 32),
          new THREE.MeshBasicMaterial({
            color: '#ffffff',
            transparent: true,
            opacity: 0.45,
            depthWrite: false,
            side: THREE.DoubleSide,
          }),
        );
        ghost.rotation.x = -Math.PI / 2;
        ghost.position.set(from[0], 0.05, from[1]);
        this.overlayRoot.add(ghost);
        if (sel?.type === 'object' && sel.id === obj.id) {
          this.overlayRoot.add(
            handle(curveMid(from, to, bend), { kind: 'bend', id: obj.id }, '#fde047'),
          );
        }
      }
    }

    if (sel?.type === 'drawing') {
      const d = this.drawings.find((x) => x.id === sel.id);
      if (d) {
        const outline = drawingPath(d);
        if (outline.length > 1) {
          this.overlayRoot.add(
            new THREE.Mesh(
              ribbon([outline], (d.width ?? DEFAULT_STROKE) + 0.5, 0.02),
              new THREE.MeshBasicMaterial({
                color: ACCENT,
                transparent: true,
                opacity: 0.35,
                depthWrite: false,
                side: THREE.DoubleSide,
              }),
            ),
          );
        }
        if (d.kind !== 'pen') {
          d.points.forEach((p, index) =>
            this.overlayRoot.add(handle(p, { kind: 'point', id: d.id, index })),
          );
        }
        if ((d.kind === 'arrow' || d.kind === 'line') && d.points.length >= 2) {
          const a = d.points[0];
          const b = d.points[d.points.length - 1];
          this.overlayRoot.add(
            handle(curveMid(a, b, d.control), { kind: 'control', id: d.id }, '#fde047'),
          );
        }
      }
    }
  }

  // --- pointer --------------------------------------------------------------

  private applyInteractivity(): void {
    const c = this.controls;
    if (this.edit.editor) {
      // Left is for editing; right orbits, wheel zooms, two fingers do both.
      c.mouseButtons = {
        LEFT: null,
        MIDDLE: THREE.MOUSE.DOLLY,
        RIGHT: THREE.MOUSE.ROTATE,
      } as unknown as typeof c.mouseButtons;
      c.touches = { ONE: null, TWO: THREE.TOUCH.DOLLY_ROTATE } as unknown as typeof c.touches;
    } else {
      c.mouseButtons = {
        LEFT: THREE.MOUSE.ROTATE,
        MIDDLE: THREE.MOUSE.DOLLY,
        RIGHT: THREE.MOUSE.PAN,
      };
      c.touches = { ONE: THREE.TOUCH.ROTATE, TWO: THREE.TOUCH.DOLLY_PAN };
    }
  }

  private pointer(e: PointerEvent | MouseEvent): THREE.Vector2 {
    const r = this.canvas.getBoundingClientRect();
    return new THREE.Vector2(
      ((e.clientX - r.left) / r.width) * 2 - 1,
      -((e.clientY - r.top) / r.height) * 2 + 1,
    );
  }

  private groundPoint(e: PointerEvent | MouseEvent): Vec2 | null {
    this.raycaster.setFromCamera(this.pointer(e), this.camera);
    const hit = new THREE.Vector3();
    if (!this.raycaster.ray.intersectPlane(this.ground, hit)) return null;
    return [hit.x, hit.z];
  }

  private hitHandle(): Handle | null {
    const hits = this.raycaster.intersectObjects(this.overlayRoot.children, true);
    for (const h of hits) {
      const handle = h.object.userData['handle'] as Handle | undefined;
      if (handle) return handle;
    }
    return null;
  }

  /**
   * The piece under the pointer. A small piece — a ball, a disc — is a few
   * pixels across on a full pitch, and on a touch screen a finger covers more
   * than that, so a miss falls back to the nearest piece within reach of
   * where the pointer meets the grass.
   */
  private hitObject(at?: Vec2): string | null {
    const hits = this.raycaster.intersectObjects(
      this.piecesRoot.children.filter((c) => c.visible),
      true,
    );
    for (const h of hits) {
      const id = h.object.userData['objectId'] as string | undefined;
      if (id) return id;
    }
    if (!at) return null;
    let best: string | null = null;
    let bestDist = 1.6;
    for (const [id, piece] of this.pieces) {
      if (!piece.group.visible) continue;
      const d = Math.hypot(piece.group.position.x - at[0], piece.group.position.z - at[1]);
      if (d < bestDist) {
        best = id;
        bestDist = d;
      }
    }
    return best;
  }

  /** Strokes are thin, so they are hit-tested on the plane with some slack. */
  private hitDrawing(at: Vec2): string | null {
    for (let i = this.drawings.length - 1; i >= 0; i--) {
      const d = this.drawings[i];
      if (d.kind === 'text') {
        if (d.points[0] && Math.hypot(at[0] - d.points[0][0], at[1] - d.points[0][1]) < 2.5)
          return d.id;
        continue;
      }
      const path = drawingPath(d);
      if (distToPolyline(at, path) < (d.width ?? DEFAULT_STROKE) / 2 + 0.9) return d.id;
      if (
        (d.kind === 'rect' || d.kind === 'ellipse' || d.kind === 'polygon') &&
        insidePolygon(at, path)
      )
        return d.id;
    }
    return null;
  }

  private readonly onPointerDown = (e: PointerEvent) => {
    if (!this.edit.editing || e.button !== 0) return;
    const at = this.groundPoint(e);
    if (!at) return;
    this.canvas.setPointerCapture(e.pointerId);

    if (this.tool === 'place' || this.tool === 'text') {
      this.handlers.place?.(clampToArea(at));
      return;
    }

    if (this.tool === 'polygon') {
      const first = this.polygon[0];
      if (
        first &&
        this.polygon.length >= 3 &&
        Math.hypot(at[0] - first[0], at[1] - first[1]) < 1.5
      ) {
        this.finishPolygon();
      } else {
        this.polygon.push(clampToArea(at));
        this.handlers.stroke?.([...this.polygon], false);
      }
      return;
    }

    if (this.tool !== 'select') {
      this.gesture = { kind: 'stroke', points: [clampToArea(at)] };
      this.handlers.stroke?.([clampToArea(at)], false);
      return;
    }

    const handle = this.hitHandle();
    if (handle) {
      this.gesture = { kind: 'handle', handle };
      return;
    }
    const objectId = this.hitObject(at);
    if (objectId && this.doc) {
      const p = positionAt(this.doc, this.edit.step, objectId) ?? at;
      this.gesture = {
        kind: 'object',
        id: objectId,
        offset: [p[0] - at[0], p[1] - at[1]],
        moved: false,
      };
      this.handlers.select?.({ type: 'object', id: objectId });
      return;
    }
    const drawingId = this.hitDrawing(at);
    if (drawingId) {
      this.gesture = { kind: 'drawing', id: drawingId, last: at, moved: false };
      this.handlers.select?.({ type: 'drawing', id: drawingId });
      return;
    }
    this.handlers.select?.(null);
  };

  private readonly onPointerMove = (e: PointerEvent) => {
    if (!this.edit.editing) return;
    const at = this.groundPoint(e);
    if (!at) return;
    const g = this.gesture;

    if (!g) {
      if (this.tool === 'select') {
        this.raycaster.setFromCamera(this.pointer(e), this.camera);
        const over = this.hitHandle() || this.hitObject(at) || this.hitDrawing(at);
        this.canvas.style.cursor = over ? 'grab' : 'default';
      } else {
        this.canvas.style.cursor = 'crosshair';
        if (this.tool === 'polygon' && this.polygon.length)
          this.handlers.stroke?.([...this.polygon, clampToArea(at)], false);
      }
      return;
    }

    this.canvas.style.cursor = g.kind === 'stroke' ? 'crosshair' : 'grabbing';
    switch (g.kind) {
      case 'object':
        g.moved = true;
        this.handlers.moveObject?.(
          g.id,
          clampToArea([at[0] + g.offset[0], at[1] + g.offset[1]]),
          false,
        );
        break;
      case 'handle':
        this.dragHandle(g.handle, clampToArea(at), false);
        break;
      case 'drawing': {
        const offset: Vec2 = [at[0] - g.last[0], at[1] - g.last[1]];
        g.last = at;
        g.moved = true;
        this.handlers.editDrawing?.(g.id, { offset }, false);
        break;
      }
      case 'stroke': {
        const p = clampToArea(at);
        const kind = this.tool;
        if (kind === 'pen') {
          const last = g.points[g.points.length - 1];
          if (Math.hypot(p[0] - last[0], p[1] - last[1]) > 0.25) g.points.push(p);
        } else {
          g.points = [g.points[0], p];
        }
        this.handlers.stroke?.([...g.points], false);
        break;
      }
    }
  };

  private readonly onPointerUp = (e: PointerEvent) => {
    const g = this.gesture;
    this.gesture = null;
    if (this.canvas.hasPointerCapture(e.pointerId)) this.canvas.releasePointerCapture(e.pointerId);
    if (!g) return;
    const at = this.groundPoint(e);
    switch (g.kind) {
      case 'object':
        if (g.moved && at)
          this.handlers.moveObject?.(
            g.id,
            clampToArea([at[0] + g.offset[0], at[1] + g.offset[1]]),
            true,
          );
        break;
      case 'handle':
        if (at) this.dragHandle(g.handle, clampToArea(at), true);
        break;
      case 'drawing':
        if (g.moved) this.handlers.editDrawing?.(g.id, { offset: [0, 0] }, true);
        break;
      case 'stroke':
        this.handlers.stroke?.(g.points, true);
        break;
    }
  };

  private readonly onDoubleClick = () => {
    if (this.tool === 'polygon' && this.polygon.length >= 3) this.finishPolygon();
  };

  private finishPolygon(): void {
    const points = this.polygon;
    this.polygon = [];
    this.handlers.stroke?.(points, true);
  }

  private dragHandle(h: Handle, at: Vec2, done: boolean): void {
    if (h.kind === 'bend') this.handlers.bend?.(h.id, at, done);
    else if (h.kind === 'point') this.handlers.editDrawing?.(h.id, { index: h.index, at }, done);
    else this.handlers.editDrawing?.(h.id, { control: at }, done);
  }

  // --- loop -----------------------------------------------------------------

  private fit(): void {
    const w = Math.max(1, this.host.clientWidth);
    const h = Math.max(1, this.host.clientHeight);
    this.renderer.setSize(w, h, false);
    this.camera.aspect = w / h;
    this.camera.updateProjectionMatrix();
  }

  private loop = () => {
    if (this.disposed) return;
    this.frameId = requestAnimationFrame(this.loop);
    const dt = Math.min(0.1, this.clock.getDelta());
    this.onTick?.(dt);

    if (this.tween) {
      this.tween.t = Math.min(1, this.tween.t + dt / 0.85);
      const k = easeInOut(this.tween.t);
      this.camera.position.lerpVectors(this.tween.from[0], this.tween.to[0], k);
      this.controls.target.lerpVectors(this.tween.from[1], this.tween.to[1], k);
      if (this.tween.t >= 1) this.tween = null;
    }
    this.controls.update();

    if (this.selectionRing.visible) {
      const t = this.clock.elapsedTime;
      (this.selectionRing.material as THREE.MeshBasicMaterial).opacity =
        0.6 + Math.sin(t * 4) * 0.25;
    }
    // Number labels stay upright and constant on screen as the camera zooms.
    this.renderer.render(this.scene, this.camera);
  };

  /** Rolls the ball by the distance it travelled since the last frame. */
  private spin(piece: { last?: Vec2 }, g: THREE.Group, at: Vec2): void {
    const ball = g.getObjectByName('ball');
    if (!ball || !piece.last) return;
    const dx = at[0] - piece.last[0];
    const dy = at[1] - piece.last[1];
    const d = Math.hypot(dx, dy);
    if (d < 1e-4 || d > 20) return;
    const axis = new THREE.Vector3(dy, 0, -dx).normalize();
    ball.rotateOnWorldAxis(axis, d / 0.42);
  }

  private clear(root: THREE.Object3D): void {
    for (const child of [...root.children]) {
      root.remove(child);
      disposeGroup(child);
    }
  }
}

function radiusOf(obj: BoardObject | undefined): number {
  if (!obj) return 1;
  switch (obj.kind) {
    case 'player':
      return PLAYER_RADIUS / 0.95;
    case 'goal':
      return 4.2;
    case 'minigoal':
    case 'ladder':
      return 2;
    case 'popup':
      return 1.1;
    default:
      return 0.6;
  }
}

function handle(at: Vec2, data: Handle, color = '#ffffff'): THREE.Mesh {
  const m = new THREE.Mesh(
    new THREE.SphereGeometry(0.55, 18, 12),
    new THREE.MeshBasicMaterial({ color, depthTest: false, transparent: true }),
  );
  m.position.set(at[0], 0.6, at[1]);
  m.renderOrder = 20;
  m.userData['handle'] = data;
  return m;
}

/** The sky behind everything: a vertical gradient painted once per style. */
function skyTexture(style: PitchSettings['style']): THREE.Texture {
  const c = document.createElement('canvas');
  c.width = 4;
  c.height = 256;
  const g = c.getContext('2d')!;
  const grad = g.createLinearGradient(0, 0, 0, 256);
  if (style === 'night') {
    grad.addColorStop(0, '#02040a');
    grad.addColorStop(0.6, '#0b1430');
    grad.addColorStop(1, '#1b2747');
  } else if (style === 'board') {
    grad.addColorStop(0, '#0f1f18');
    grad.addColorStop(1, '#183126');
  } else {
    grad.addColorStop(0, '#5b9bd5');
    grad.addColorStop(0.55, '#a9cbe8');
    grad.addColorStop(1, '#e3edf5');
  }
  g.fillStyle = grad;
  g.fillRect(0, 0, 4, 256);
  const t = new THREE.CanvasTexture(c);
  t.colorSpace = THREE.SRGBColorSpace;
  return t;
}

/**
 * Low stands on all four sides and, at night, floodlight towers. Simple
 * blocks, but they are what make the tilted views read as a stadium rather
 * than a green rectangle floating in the sky.
 */
function stadium(night: boolean): THREE.Group {
  const g = new THREE.Group();
  const concrete = new THREE.MeshStandardMaterial({
    color: night ? '#2a313a' : '#9aa5b1',
    roughness: 0.9,
  });
  const seats = [
    new THREE.MeshStandardMaterial({ color: night ? '#1e3a5f' : '#1e40af', roughness: 0.7 }),
    new THREE.MeshStandardMaterial({ color: night ? '#24476f' : '#2563eb', roughness: 0.7 }),
  ];
  const edgeL = HALF_LENGTH + RUNOFF + 2;
  const edgeW = HALF_WIDTH + RUNOFF + 2;
  const tiers = 7;

  const side = (length: number, depthAxis: 'x' | 'z', sign: number, offset: number) => {
    for (let i = 0; i < tiers; i++) {
      const h = 1.1 + i * 1.25;
      const geo =
        depthAxis === 'z'
          ? new THREE.BoxGeometry(length, h, 2.2)
          : new THREE.BoxGeometry(2.2, h, length);
      const block = new THREE.Mesh(geo, i % 2 ? seats[0] : seats[1]);
      const out = offset + 1.1 + i * 2.2;
      if (depthAxis === 'z') block.position.set(0, h / 2, sign * out);
      else block.position.set(sign * out, h / 2, 0);
      block.receiveShadow = true;
      block.castShadow = i === tiers - 1;
      g.add(block);
    }
    const wallH = 1.2 + tiers * 1.25 + 2.5;
    const back =
      depthAxis === 'z'
        ? new THREE.BoxGeometry(length + 4, wallH, 1)
        : new THREE.BoxGeometry(1, wallH, length + 4);
    const wall = new THREE.Mesh(back, concrete);
    const out = offset + 1.1 + tiers * 2.2 + 0.5;
    if (depthAxis === 'z') wall.position.set(0, wallH / 2, sign * out);
    else wall.position.set(sign * out, wallH / 2, 0);
    g.add(wall);
  };
  // No stand on the near (+z) side: every preset camera looks from there, and
  // a stand would sit between it and the pitch. The far stand runs past the
  // ends so it closes the corners behind the end stands.
  const depth = tiers * 2.2 + 1.1;
  side((edgeL + depth) * 2, 'z', -1, edgeW);
  side(edgeW * 2, 'x', 1, edgeL);
  side(edgeW * 2, 'x', -1, edgeL);

  // Advertising boards along the far touchline.
  const boardMat = new THREE.MeshStandardMaterial({
    color: night ? '#0ea5e9' : '#0f766e',
    emissive: night ? '#0369a1' : '#000000',
    emissiveIntensity: night ? 0.6 : 0,
    roughness: 0.4,
  });
  const ad = new THREE.Mesh(new THREE.BoxGeometry(HALF_LENGTH * 1.7, 0.9, 0.15), boardMat);
  ad.position.set(0, 0.45, -(HALF_WIDTH + 3.2));
  ad.castShadow = true;
  g.add(ad);

  if (night) {
    const lamp = new THREE.MeshStandardMaterial({
      color: '#ffffff',
      emissive: '#fff8dc',
      emissiveIntensity: 2.2,
    });
    const mast = new THREE.MeshStandardMaterial({ color: '#4b5563', roughness: 0.6 });
    for (const sx of [-1, 1]) {
      for (const sz of [-1, 1]) {
        const x = sx * (edgeL + 14);
        const z = sz * (edgeW + 14);
        const pole = new THREE.Mesh(new THREE.CylinderGeometry(0.5, 0.8, 42, 10), mast);
        pole.position.set(x, 21, z);
        const panel = new THREE.Mesh(new THREE.BoxGeometry(7, 4, 0.6), lamp);
        panel.position.set(x, 43, z);
        panel.lookAt(0, 0, 0);
        g.add(pole, panel);
      }
    }
  }
  return g;
}
