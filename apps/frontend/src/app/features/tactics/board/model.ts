/**
 * The tactics board document.
 *
 * A board is stored as data, not video: which players, cones and goals exist,
 * where each one stands at every step, and what is drawn on the grass. The
 * browser animates it. That is why a whole drill costs tens of kilobytes in a
 * table the API already has, and why it can be edited after it is shared.
 *
 * **Coordinates are metres on a 105 × 68 pitch, origin at the centre spot.**
 * `x` runs along the length (home goal at −52.5, away goal at +52.5) and `y`
 * across it. Metres rather than pixels so a board looks the same on a phone
 * and on a projector, and so a 20-metre rondo grid is 20 of something real.
 *
 * The document carries a version. A newer editor reads an older board through
 * `normalizeDoc`, which is the one place an old shape is upgraded.
 */

export const PITCH_LENGTH = 105;
export const PITCH_WIDTH = 68;
export const HALF_LENGTH = PITCH_LENGTH / 2;
export const HALF_WIDTH = PITCH_WIDTH / 2;

/** How far past the touchlines something may be placed: the runoff. */
export const RUNOFF = 6;

/** Default stroke width for drawings, in metres. */
export const DEFAULT_STROKE = 0.32;

export type Vec2 = [number, number];

export type TeamId = 'home' | 'away' | 'third';
export const TEAM_IDS: readonly TeamId[] = ['home', 'away', 'third'];

export type KitPattern = 'solid' | 'stripes' | 'hoops' | 'halves' | 'sash';
export const KIT_PATTERNS: readonly KitPattern[] = ['solid', 'stripes', 'hoops', 'halves', 'sash'];

export interface Kit {
  name: string;
  primary: string;
  secondary: string;
  pattern: KitPattern;
  /** Shirt numbers. */
  numbers: string;
  /** Goalkeepers wear their own colour. */
  keeper: string;
}

export type EquipmentKind =
  | 'ball'
  | 'cone'
  | 'disc'
  | 'pole'
  | 'mannequin'
  | 'hurdle'
  | 'ladder'
  | 'ring'
  | 'minigoal'
  | 'goal'
  | 'popup'
  | 'flag';

export type ObjectKind = 'player' | EquipmentKind;

export interface BoardObject {
  id: string;
  kind: ObjectKind;

  // --- players ---
  team?: TeamId;
  keeper?: boolean;
  number?: string;
  name?: string;
  /**
   * A training bib over the kit. This is how a neutral player is shown in a
   * rondo: same team kit, different bib.
   */
  bib?: string;

  /**
   * Overrides. For a player these replace the team kit's colours for this one
   * player; for equipment `color` is simply its colour.
   */
  color?: string;
  color2?: string;
  pattern?: KitPattern;

  /** Scale, 1 = default. */
  size?: number;
  /** Facing, in degrees, about the vertical axis. Goals and mannequins use it. */
  rotation?: number;
}

export type DrawingKind = 'arrow' | 'line' | 'pen' | 'rect' | 'ellipse' | 'polygon' | 'text';

/**
 * Line styles follow the usual coaching notation, so a board reads the way a
 * coach expects: solid for a run, dashed for a pass, wavy for a dribble.
 */
export type StrokeStyle = 'solid' | 'dashed' | 'wavy';

export interface Drawing {
  id: string;
  kind: DrawingKind;
  points: Vec2[];
  /** Bends an arrow or line into a curve; absent means straight. */
  control?: Vec2;
  style?: StrokeStyle;
  color: string;
  /** Stroke width in metres. */
  width?: number;
  /** For text. */
  text?: string;
  /** Zones are filled; this is the fill's opacity. */
  fill?: number;
}

export interface Step {
  id: string;
  /** Seconds to travel from the previous step into this one. */
  duration: number;
  /** Seconds to pause on this step before moving on. */
  hold: number;
  /** A caption shown while this step plays. */
  note: string;
  /**
   * Where each object stands at this step. An object missing here stands
   * where it stood in the previous step — so adding a step copies nothing and
   * changes nothing until something is moved.
   */
  pos: Record<string, Vec2>;
  /** A curve for the move into this step, by object id. */
  bend?: Record<string, Vec2>;
  /** Objects travelling through the air into this step: a lofted pass. */
  air?: string[];
  /** Objects picked out with a spotlight during this step. */
  highlight?: string[];
  drawings: Drawing[];
}

export type PitchLayout = 'full' | 'half' | 'third' | 'box' | 'drill';
export type PitchMarkings = 'standard' | 'none' | 'grid';
export type PitchStyle = 'day' | 'night' | 'board';

export interface PitchSettings {
  layout: PitchLayout;
  markings: PitchMarkings;
  style: PitchStyle;
  /** Stands around the pitch. Off for a cleaner analysis view. */
  stadium: boolean;
}

export interface TacticDoc {
  v: 1;
  pitch: PitchSettings;
  teams: Record<TeamId, Kit>;
  objects: BoardObject[];
  steps: Step[];
}

// --- defaults ------------------------------------------------------------

export const DEFAULT_KITS: Record<TeamId, Kit> = {
  home: {
    name: 'Home',
    primary: '#c8102e',
    secondary: '#ffffff',
    pattern: 'solid',
    numbers: '#ffffff',
    keeper: '#1f9d55',
  },
  away: {
    name: 'Away',
    primary: '#1d4ed8',
    secondary: '#0b1f4d',
    pattern: 'solid',
    numbers: '#ffffff',
    keeper: '#f59e0b',
  },
  third: {
    name: 'Neutral',
    primary: '#facc15',
    secondary: '#111827',
    pattern: 'solid',
    numbers: '#111827',
    keeper: '#a855f7',
  },
};

/** Default colour of each piece of equipment when placed. */
export const EQUIPMENT_COLORS: Record<EquipmentKind, string> = {
  ball: '#ffffff',
  cone: '#ff7a1a',
  disc: '#facc15',
  pole: '#f43f5e',
  mannequin: '#2563eb',
  hurdle: '#f97316',
  ladder: '#fde047',
  ring: '#22d3ee',
  minigoal: '#f8fafc',
  goal: '#f8fafc',
  popup: '#f97316',
  flag: '#facc15',
};

/** The swatches offered everywhere a colour can be picked. */
export const PALETTE: readonly string[] = [
  '#ffffff',
  '#111827',
  '#c8102e',
  '#f43f5e',
  '#f97316',
  '#ff7a1a',
  '#facc15',
  '#a3e635',
  '#22c55e',
  '#14b8a6',
  '#22d3ee',
  '#3b82f6',
  '#1d4ed8',
  '#7c3aed',
  '#ec4899',
  '#94a3b8',
];

export function emptyDoc(): TacticDoc {
  return {
    v: 1,
    pitch: { layout: 'full', markings: 'standard', style: 'day', stadium: true },
    teams: structuredClone(DEFAULT_KITS),
    objects: [],
    steps: [newStep()],
  };
}

export function newStep(): Step {
  return { id: newId(), duration: 1.6, hold: 0.6, note: '', pos: {}, drawings: [] };
}

/**
 * A short random id, unique within one board. Not a UUID: there are at most a
 * few hundred things on a board, and every id is repeated in every step.
 */
export function newId(): string {
  const bytes = new Uint8Array(6);
  globalThis.crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => (b % 36).toString(36)).join('');
}

// --- reading positions ---------------------------------------------------

/**
 * Where an object stands at a step: its position in that step, or the latest
 * earlier step that placed it. Undefined means it has not appeared yet.
 */
export function positionAt(doc: TacticDoc, stepIndex: number, id: string): Vec2 | undefined {
  for (let i = Math.min(stepIndex, doc.steps.length - 1); i >= 0; i--) {
    const p = doc.steps[i].pos[id];
    if (p) return p;
  }
  return undefined;
}

/** Every object present at a step, with where it stands. */
export function positionsAt(doc: TacticDoc, stepIndex: number): Map<string, Vec2> {
  const out = new Map<string, Vec2>();
  for (const obj of doc.objects) {
    const p = positionAt(doc, stepIndex, obj.id);
    if (p) out.set(obj.id, p);
  }
  return out;
}

/** Whether an object moved into this step, which is what gets a path drawn. */
export function movedInto(doc: TacticDoc, stepIndex: number, id: string): Vec2 | undefined {
  if (stepIndex === 0) return undefined;
  const now = doc.steps[stepIndex].pos[id];
  const before = positionAt(doc, stepIndex - 1, id);
  if (!now || !before) return undefined;
  return Math.hypot(now[0] - before[0], now[1] - before[1]) > 0.05 ? before : undefined;
}

/** The colours a player is drawn in, after the team kit and any overrides. */
export function playerLook(doc: TacticDoc, obj: BoardObject) {
  const kit = doc.teams[obj.team ?? 'home'];
  return {
    primary: obj.color ?? (obj.keeper ? kit.keeper : kit.primary),
    secondary: obj.color2 ?? kit.secondary,
    pattern: obj.pattern ?? (obj.keeper ? 'solid' : kit.pattern),
    numbers: kit.numbers,
  };
}

/** The next free shirt number in a team. */
export function nextNumber(doc: TacticDoc, team: TeamId): string {
  const used = new Set(
    doc.objects.filter((o) => o.kind === 'player' && o.team === team).map((o) => o.number),
  );
  for (let n = 1; n < 100; n++) if (!used.has(String(n))) return String(n);
  return '';
}

export function clampToArea([x, y]: Vec2): Vec2 {
  const maxX = HALF_LENGTH + RUNOFF;
  const maxY = HALF_WIDTH + RUNOFF;
  return [Math.max(-maxX, Math.min(maxX, x)), Math.max(-maxY, Math.min(maxY, y))];
}

// --- formations ----------------------------------------------------------

/**
 * Formations as (depth, width) pairs: depth is how far back from the halfway
 * line (0 is a striker on it, 1 is the own box); width 0 is the top touchline
 * and 1 the bottom. Mirrored for the side defending the right-hand goal.
 */
const FORMATIONS: Record<string, { n: string; at: Vec2 }[]> = {
  '4-4-2': shape(
    [
      [0.82, 0.12],
      [0.8, 0.37],
      [0.8, 0.63],
      [0.82, 0.88],
    ],
    [
      [0.45, 0.12],
      [0.42, 0.38],
      [0.42, 0.62],
      [0.45, 0.88],
    ],
    [
      [0.12, 0.38],
      [0.12, 0.62],
    ],
    ['2', '5', '6', '3', '7', '4', '8', '11', '9', '10'],
  ),
  '4-3-3': shape(
    [
      [0.82, 0.12],
      [0.8, 0.37],
      [0.8, 0.63],
      [0.82, 0.88],
    ],
    [
      [0.55, 0.5],
      [0.4, 0.3],
      [0.4, 0.7],
    ],
    [
      [0.14, 0.15],
      [0.08, 0.5],
      [0.14, 0.85],
    ],
    ['2', '5', '4', '3', '6', '8', '10', '7', '9', '11'],
  ),
  '4-2-3-1': shape(
    [
      [0.82, 0.12],
      [0.8, 0.37],
      [0.8, 0.63],
      [0.82, 0.88],
    ],
    [
      [0.58, 0.38],
      [0.58, 0.62],
    ],
    [
      [0.32, 0.15],
      [0.3, 0.5],
      [0.32, 0.85],
    ],
    [[0.08, 0.5]],
    ['2', '5', '4', '3', '6', '8', '7', '10', '11', '9'],
  ),
  '3-5-2': shape(
    [
      [0.8, 0.25],
      [0.82, 0.5],
      [0.8, 0.75],
    ],
    [
      [0.45, 0.08],
      [0.52, 0.35],
      [0.58, 0.5],
      [0.52, 0.65],
      [0.45, 0.92],
    ],
    [
      [0.12, 0.38],
      [0.12, 0.62],
    ],
    ['4', '5', '6', '2', '8', '10', '7', '3', '9', '11'],
  ),
  '3-4-3': shape(
    [
      [0.8, 0.25],
      [0.82, 0.5],
      [0.8, 0.75],
    ],
    [
      [0.48, 0.1],
      [0.52, 0.38],
      [0.52, 0.62],
      [0.48, 0.9],
    ],
    [
      [0.15, 0.18],
      [0.08, 0.5],
      [0.15, 0.82],
    ],
    ['4', '5', '6', '2', '8', '10', '3', '7', '9', '11'],
  ),
  '5-3-2': shape(
    [
      [0.72, 0.08],
      [0.82, 0.3],
      [0.84, 0.5],
      [0.82, 0.7],
      [0.72, 0.92],
    ],
    [
      [0.5, 0.3],
      [0.55, 0.5],
      [0.5, 0.7],
    ],
    [
      [0.14, 0.38],
      [0.14, 0.62],
    ],
    ['2', '4', '5', '6', '3', '8', '10', '7', '9', '11'],
  ),
};

/** Rows of spots, defence first, then the shirt numbers in the same order. */
function shape(...rows: (Vec2[] | string[])[]): { n: string; at: Vec2 }[] {
  const numbers = rows[rows.length - 1] as string[];
  const spots = (rows.slice(0, -1) as Vec2[][]).flat();
  return spots.map((at, i) => ({ n: numbers[i], at }));
}

export const FORMATION_NAMES = Object.keys(FORMATIONS);

/**
 * Lays out a whole team: a goalkeeper and ten outfield players in the chosen
 * shape, in that team's half. Returns the new objects and where they stand.
 */
export function formation(
  name: string,
  team: TeamId,
  side: 'left' | 'right',
): { objects: BoardObject[]; pos: Record<string, Vec2> } {
  const spots = FORMATIONS[name] ?? FORMATIONS['4-4-2'];
  const dir = side === 'left' ? 1 : -1;
  const objects: BoardObject[] = [];
  const pos: Record<string, Vec2> = {};

  const keeper: BoardObject = { id: newId(), kind: 'player', team, keeper: true, number: '1' };
  objects.push(keeper);
  pos[keeper.id] = [-dir * (HALF_LENGTH - 4), 0];

  for (const spot of spots) {
    const obj: BoardObject = { id: newId(), kind: 'player', team, number: spot.n };
    objects.push(obj);
    // The rows give depth from the halfway line (0.8 is a defender), so it is
    // inverted to measure from the own goal.
    const depth = 1 - spot.at[0];
    const across = spot.at[1];
    // Outfield players use the half from just outside the box to near halfway.
    const x = -dir * (HALF_LENGTH - 10 - depth * (HALF_LENGTH - 14));
    const y = (across - 0.5) * (PITCH_WIDTH - 10);
    pos[obj.id] = [round(x), round(y)];
  }
  return { objects, pos };
}

function round(n: number): number {
  return Math.round(n * 10) / 10;
}

// --- reading untrusted documents -----------------------------------------

/**
 * Turns whatever came back from the API into a document this editor can use.
 *
 * A board is somebody else's JSON by the time it is opened: it may predate a
 * field, or have been written by a buggy client. Rather than trusting it, each
 * part is taken if well-formed and defaulted if not, so a damaged board opens
 * with what can be saved rather than not opening at all.
 */
export function normalizeDoc(raw: unknown): TacticDoc {
  const base = emptyDoc();
  if (!raw || typeof raw !== 'object') return base;
  const src = raw as Partial<TacticDoc>;

  const pitch = { ...base.pitch, ...(isObject(src.pitch) ? src.pitch : {}) };
  const teams = { ...base.teams };
  if (isObject(src.teams)) {
    for (const id of TEAM_IDS) {
      const kit = (src.teams as Record<string, unknown>)[id];
      if (isObject(kit)) teams[id] = { ...base.teams[id], ...(kit as Partial<Kit>) };
    }
  }

  const objects = Array.isArray(src.objects)
    ? src.objects.filter(
        (o): o is BoardObject =>
          isObject(o) && typeof o.id === 'string' && typeof o.kind === 'string',
      )
    : [];

  const steps = Array.isArray(src.steps)
    ? src.steps.filter(isObject).map((s) => ({
        ...newStep(),
        ...(s as Partial<Step>),
        id: typeof s['id'] === 'string' ? (s['id'] as string) : newId(),
        pos: isObject(s['pos']) ? (s['pos'] as Record<string, Vec2>) : {},
        drawings: Array.isArray(s['drawings']) ? (s['drawings'] as Drawing[]) : [],
      }))
    : [];

  return { v: 1, pitch, teams, objects, steps: steps.length ? steps : base.steps };
}

function isObject(v: unknown): v is Record<string, unknown> {
  return !!v && typeof v === 'object' && !Array.isArray(v);
}
