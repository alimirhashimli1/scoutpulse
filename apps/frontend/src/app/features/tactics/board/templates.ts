/**
 * Ready-made boards a member can open in the editor and save as their own.
 *
 * Opened at `/tactics/new?template=<key>`. A template is only a starting
 * document: nothing is saved until the member presses Save, and from then on
 * it is an ordinary board of theirs.
 */
import { DEFAULT_KITS, Drawing, HALF_LENGTH, Step, TacticDoc, Vec2 } from './model';

export interface BoardTemplate {
  title: string;
  description: string;
  doc: () => TacticDoc;
}

export const TEMPLATES: Record<string, BoardTemplate> = {
  'pressing-drill-1': {
    title: 'Pressing drill - 1',
    description:
      '4 v 2 in a small box on the edge of the area, turning into a counter-press and a ' +
      'quick attack on the real goal. Red: 6 players, 2 inside and 4 waiting outside. ' +
      'Blue: 4 players, attack the goal. Plus a keeper.',
    doc: pressingDrill1,
  },
  'wing-cross-dummy-1': {
    title: 'Wing cross and dummy - 1',
    description:
      'Three players: central midfielder, left winger and centre forward. The midfielder ' +
      'first makes a recovery sprint to raise his pulse, then has to combine on tired legs. ' +
      'A long ball to the winger, a run down the line, a pull-back from the byline, a dummy ' +
      'from the centre forward and a late finish from the midfielder.',
    doc: wingCrossDummy1,
  },
};

/**
 * Spots measured from the goal line: `across` metres from the middle (positive
 * is below the middle on screen) and `deep` metres out from the goal at +x.
 */
function at(across: number, deep: number): Vec2 {
  return [Math.round((HALF_LENGTH - deep) * 10) / 10, across];
}

function pressingDrill1(): TacticDoc {
  const RED = '#dc2626';
  const BLUE = '#2563eb';
  const ORANGE = '#e8891c';

  const reds = ['r1', 'r2', 'r3', 'r4', 'r5', 'r6'];
  const blues = ['b1', 'b2', 'b3', 'b4'];

  // What stays on the grass the whole drill: the two boxes and their labels.
  const fixed: Drawing[] = [
    {
      id: 'bigbox',
      kind: 'rect',
      points: [at(-15, 8.5), at(15, 32.5)],
      color: ORANGE,
      width: 0.34,
      fill: 0,
    },
    {
      id: 'smallbox',
      kind: 'rect',
      points: [at(-6, 15.5), at(6, 25.5)],
      color: RED,
      width: 0.3,
      fill: 0.06,
    },
    {
      id: 'smalllabel',
      kind: 'text',
      points: [at(7.6, 20.5)],
      text: 'small box 12 × 10 m',
      color: '#f1f5f9',
      width: 0.16,
    },
    {
      id: 'biglabel',
      kind: 'text',
      points: [at(13.4, 26.5)],
      text: 'BIG BOX 30 × 24 m',
      color: '#f1f5f9',
      width: 0.18,
    },
    {
      id: 'playerslabel',
      kind: 'text',
      points: [at(17.6, 20.5)],
      text: '10 players + a keeper in the real goal',
      color: '#f1f5f9',
      width: 0.18,
    },
  ];

  /** A big headline across the middle of the drill, for one step only. */
  const banner = (id: string, text: string): Drawing => ({
    id,
    kind: 'text',
    points: [at(0, 22.5)],
    text,
    color: '#ffffff',
    width: 0.7,
  });

  const step = (
    n: number,
    note: string,
    pos: Record<string, Vec2>,
    extra: Drawing[] = [],
  ): Step => ({
    id: `s${n}`,
    duration: 1.8,
    hold: 2.6,
    note,
    pos,
    drawings: [...fixed, ...extra],
  });

  const steps: Step[] = [
    step(
      1,
      'Phase 1: 4 v 2 in the small box on the edge of the penalty area. Blue keeps the ball and ' +
        'two reds press. The other four reds wait in the big box. A keeper stands in the real goal.',
      {
        gk: at(0, 1.5),
        r1: at(-11.5, 11.3),
        r2: at(11.3, 11.3),
        r3: at(-1.8, 19.4),
        r4: at(1.8, 22.7),
        r5: at(-11.6, 28.5),
        r6: at(11.8, 30),
        b1: at(0, 16.6),
        b2: at(-5.2, 20.8),
        b3: at(5.1, 20.8),
        b4: at(0, 25.2),
        ball: at(-4.4, 21.6),
      },
    ),
    step(
      2,
      'Blue passes around the small box. One red presses the ball, the other covers the middle.',
      { ball: at(0.9, 17.4), r3: at(-1.2, 18.6), r4: at(0.8, 20.6) },
    ),
    step(3, 'The pressers swap jobs as the ball moves.', {
      ball: at(6, 21.6),
      r3: at(-0.7, 20.9),
      r4: at(2.4, 19.2),
    }),
    step(
      4,
      'A pass is intercepted. Red has the ball.',
      { r3: at(2.4, 23.3), ball: at(3.1, 23.9) },
      [banner('banner-red', 'RED WINS IT')],
    ),
    step(
      5,
      'Phase 2: red plays out to a teammate in the big box. Now it is 6 v 4, and blue ' +
        'counter-presses at once.',
      { ball: at(12.6, 31), r3: at(2.6, 25), b3: at(8.2, 26.3) },
    ),
    step(
      6,
      'Blue’s nearest player presses the ball, the second blocks the pass inside, the other ' +
        'two block the passes along the edge with their cover shadows.',
      {
        r4: at(-5.6, 18.6),
        b3: at(10, 27.5),
        b4: at(5.7, 27),
        b2: at(0, 29),
        b1: at(11.5, 20),
      },
    ),
    step(
      7,
      'The pass inside is cut out. Blue has won the ball back.',
      { ball: at(6.3, 28.2) },
      [banner('banner-won', 'WON BACK')],
    ),
    step(
      8,
      'Phase 3: blue attacks the real goal at once, with 6 seconds to shoot. Red reacts too: the ' +
        'nearest reds chase the ball, and the corner players sprint back toward the goal to ' +
        'block the shot and the passes.',
      {
        b4: at(3.5, 18.5),
        ball: at(4.4, 19.6),
        b1: at(8.2, 11.5),
        b2: at(-1.5, 22.9),
        r1: at(-4.3, 10.8),
        r2: at(10.6, 8.6),
        r3: at(4.9, 22.1),
        r4: at(-1.3, 17.1),
        r5: at(-7.1, 22.3),
        r6: at(7.4, 24.4),
      },
    ),
    step(
      9,
      'Blue is faster: a pass into the run in the penalty area before the corner reds are back.',
      { ball: at(9, 12.6) },
    ),
    step(
      10,
      'Shot and goal: 2 points. The keeper then restarts with a pass into the small box.',
      { ball: at(-1.5, -0.8), gk: at(1.6, 1.6) },
      [banner('banner-goal', 'GOAL · +2')],
    ),
  ];

  return {
    v: 1,
    pitch: { layout: 'third', markings: 'standard', style: 'day', stadium: false },
    teams: {
      home: { ...DEFAULT_KITS.home, name: 'Red', primary: RED },
      away: { ...DEFAULT_KITS.away, name: 'Blue', primary: BLUE },
      third: { ...DEFAULT_KITS.third, name: 'Keeper', keeper: ORANGE, numbers: '#ffffff' },
    },
    objects: [
      { id: 'gk', kind: 'player', team: 'third', keeper: true, number: 'GK' },
      ...reds.map((id) => ({ id, kind: 'player' as const, team: 'home' as const })),
      ...blues.map((id) => ({ id, kind: 'player' as const, team: 'away' as const })),
      { id: 'ball', kind: 'ball' },
    ],
    steps,
  };
}

function wingCrossDummy1(): TacticDoc {
  const YELLOW = '#facc15';
  const LABEL = '#f1f5f9';

  // What stays on the grass the whole drill.
  const fixed: Drawing[] = [
    {
      id: 'lane',
      kind: 'rect',
      points: [at(-34, 0), at(-24, 38)],
      color: YELLOW,
      width: 0.26,
      fill: 0.08,
    },
    {
      id: 'lanelabel',
      kind: 'text',
      points: [at(-29, 40)],
      text: 'wing lane',
      color: LABEL,
      width: 0.18,
    },
    {
      id: 'sprintlabel',
      kind: 'text',
      points: [at(9, 48.5)],
      text: 'recovery sprint',
      color: LABEL,
      width: 0.18,
    },
  ];

  const banner = (id: string, text: string, where: Vec2): Drawing => ({
    id,
    kind: 'text',
    points: [where],
    text,
    color: '#ffffff',
    width: 0.6,
  });

  const step = (
    n: number,
    note: string,
    pos: Record<string, Vec2>,
    more: Partial<Step> = {},
    extra: Drawing[] = [],
  ): Step => ({
    id: `s${n}`,
    duration: 1.8,
    hold: 2.6,
    note,
    pos,
    ...more,
    drawings: [...fixed, ...extra],
  });

  const steps: Step[] = [
    step(
      1,
      'Three players: the centre forward (9) has the ball, the central midfielder (8) stands ' +
        'behind him and the left winger (11) waits wide on the left.',
      {
        cone: at(0, 48.5),
        cf: at(0, 28),
        ball: at(0.8, 28.6),
        cm: at(0, 38),
        lw: at(-27, 36),
      },
    ),
    step(
      2,
      'The challenge first: the midfielder makes a recovery run, a full sprint back round the ' +
        'cone toward halfway. His pulse goes up.',
      { cm: at(-0.8, 47.6) },
      { duration: 2.2 },
      [banner('banner-pulse', 'PULSE UP', at(0, 42.5))],
    ),
    step(
      3,
      'He turns and sprints straight back toward the centre forward, still breathing hard.',
      { cm: at(1.5, 33.2) },
      { duration: 2.2 },
    ),
    step(
      4,
      'The centre forward plays the ball into his feet as he arrives. Tired legs make the ' +
        'first touch and the pass back much harder.',
      { ball: at(1.4, 32.4) },
      { duration: 1.2 },
    ),
    step(
      5,
      'One touch back to the centre forward. No time to rest.',
      { ball: at(-1.2, 27.4), cf: at(-1.8, 27) },
      { duration: 1.2 },
    ),
    step(
      6,
      'The centre forward sends a long ball in the air to the left winger, already running ' +
        'down the line. Then both central players run to the goal.',
      {
        ball: at(-29, 23.2),
        lw: at(-29.2, 24),
        cf: at(-2.5, 22),
        cm: at(1, 30),
      },
      { duration: 2, air: ['ball'] },
    ),
    step(
      7,
      'The winger carries the ball down the very edge of the pitch to the last line. The ' +
        'centre forward attacks the near post area; the midfielder follows a few metres behind.',
      {
        lw: at(-30, 3.2),
        ball: at(-29.4, 2.6),
        cf: at(-5, 13.5),
        cm: at(1, 21),
      },
      { duration: 2.4 },
    ),
    step(
      8,
      'From the byline the winger pulls the ball back into the centre. The centre forward ' +
        'runs at it as if to shoot...',
      {
        ball: at(-4, 10.6),
        cf: at(-3.4, 11.4),
        cm: at(2, 16.5),
        lw: at(-28.5, 3.8),
      },
      { duration: 1.3 },
    ),
    step(
      9,
      '...and lets it run through on purpose: a dummy. In a match his run drags the defenders ' +
        'with him while the ball rolls on to the midfielder arriving late.',
      { ball: at(2.8, 14.6), cf: at(-1.5, 7), cm: at(3.2, 15.4) },
      { duration: 1.1 },
      [banner('banner-dummy', 'DUMMY', at(14, 20))],
    ),
    step(
      10,
      'The midfielder finishes first time. Swap roles and start again, then run it from the ' +
        'right wing.',
      { ball: at(2, -0.8) },
      { duration: 0.8 },
      [banner('banner-goal', 'GOAL', at(0, 22))],
    ),
  ];

  return {
    v: 1,
    pitch: { layout: 'half', markings: 'standard', style: 'day', stadium: false },
    teams: structuredClone(DEFAULT_KITS),
    objects: [
      { id: 'cone', kind: 'cone', color: '#ff7a1a' },
      { id: 'cm', kind: 'player', team: 'home', number: '8', name: 'CM' },
      { id: 'lw', kind: 'player', team: 'home', number: '11', name: 'LW' },
      { id: 'cf', kind: 'player', team: 'home', number: '9', name: 'CF' },
      { id: 'ball', kind: 'ball' },
    ],
    steps,
  };
}
