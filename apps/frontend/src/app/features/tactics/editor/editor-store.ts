import { Injectable, computed, signal } from '@angular/core';

import type { DrawingChange, Target, Tool } from '../board/engine';
import { controlThrough, thin } from '../board/geometry';
import {
  BoardObject,
  DEFAULT_STROKE,
  Drawing,
  DrawingKind,
  EQUIPMENT_COLORS,
  EquipmentKind,
  Kit,
  PitchSettings,
  Step,
  StrokeStyle,
  TacticDoc,
  TeamId,
  Vec2,
  emptyDoc,
  formation,
  newId,
  newStep,
  nextNumber,
  positionAt,
  positionsAt,
} from '../board/model';
import { Frame, still } from '../board/playback';

/** What the place tool puts down. */
export type Placeable =
  { kind: 'player'; team: TeamId; keeper?: boolean } | { kind: EquipmentKind };

/** The pen: what a new drawing looks like. */
export interface Pen {
  color: string;
  style: StrokeStyle;
  width: number;
}

const HISTORY_LIMIT = 100;

/**
 * The editor's state.
 *
 * The document is treated as immutable: every change builds a new object and
 * shares everything it did not touch. That makes undo a stack of references
 * rather than copies, and lets the engine tell cheaply what changed — a
 * step's drawings array is the same array until a drawing in it changes.
 *
 * Drags are one undo entry, not sixty: the snapshot is taken when a gesture
 * starts and nothing more is recorded until it ends.
 */
@Injectable()
export class EditorStore {
  readonly doc = signal<TacticDoc>(emptyDoc());
  readonly stepIndex = signal(0);
  readonly selection = signal<Target | null>(null);
  readonly tool = signal<Tool>('select');
  readonly placing = signal<Placeable>({ kind: 'player', team: 'home' });
  readonly pen = signal<Pen>({ color: '#ffffff', style: 'solid', width: DEFAULT_STROKE });
  readonly preview = signal<Drawing | null>(null);
  /** Changed since the last save or load. */
  readonly dirty = signal(false);

  private readonly undoStack = signal<TacticDoc[]>([]);
  private readonly redoStack = signal<TacticDoc[]>([]);
  private gestureOpen = false;
  /** A text drawing just placed, whose content the inspector should focus. */
  readonly focusText = signal<string | null>(null);

  readonly canUndo = computed(() => this.undoStack().length > 0);
  readonly canRedo = computed(() => this.redoStack().length > 0);

  readonly step = computed(() => this.doc().steps[this.stepIndex()] ?? this.doc().steps[0]);

  readonly selectedObject = computed<BoardObject | null>(() => {
    const sel = this.selection();
    return sel?.type === 'object'
      ? (this.doc().objects.find((o) => o.id === sel.id) ?? null)
      : null;
  });

  readonly selectedDrawing = computed<Drawing | null>(() => {
    const sel = this.selection();
    return sel?.type === 'drawing'
      ? (this.step().drawings.find((d) => d.id === sel.id) ?? null)
      : null;
  });

  /** The board standing still at the current step, as the engine draws it. */
  readonly stillFrame = computed<Frame>(() => {
    const doc = this.doc();
    const i = this.stepIndex();
    const step = this.step();
    return {
      step: i,
      placements: still(doc, i),
      drawings: step.drawings,
      drawingOpacity: 1,
      highlights: step.highlight ?? [],
      note: step.note,
    };
  });

  load(doc: TacticDoc): void {
    this.doc.set(doc);
    this.stepIndex.set(0);
    this.selection.set(null);
    this.undoStack.set([]);
    this.redoStack.set([]);
    this.dirty.set(false);
  }

  markSaved(): void {
    this.dirty.set(false);
  }

  // --- history ------------------------------------------------------------

  /**
   * Applies a change. `phase` groups a drag: 'start'/'move' leave the gesture
   * open, 'end' closes it, and 'once' is a self-contained edit.
   */
  private apply(next: TacticDoc, phase: 'once' | 'move' | 'end' = 'once'): void {
    if (!this.gestureOpen) {
      this.undoStack.update((s) => [...s.slice(-HISTORY_LIMIT + 1), this.doc()]);
      this.redoStack.set([]);
    }
    this.gestureOpen = phase === 'move';
    this.doc.set(next);
    this.dirty.set(true);
  }

  undo(): void {
    const stack = this.undoStack();
    if (!stack.length) return;
    this.redoStack.update((s) => [...s, this.doc()]);
    this.doc.set(stack[stack.length - 1]);
    this.undoStack.set(stack.slice(0, -1));
    this.afterHistory();
  }

  redo(): void {
    const stack = this.redoStack();
    if (!stack.length) return;
    this.undoStack.update((s) => [...s, this.doc()]);
    this.doc.set(stack[stack.length - 1]);
    this.redoStack.set(stack.slice(0, -1));
    this.afterHistory();
  }

  private afterHistory(): void {
    this.gestureOpen = false;
    this.dirty.set(true);
    this.stepIndex.update((i) => Math.min(i, this.doc().steps.length - 1));
    const sel = this.selection();
    if (sel?.type === 'object' && !this.doc().objects.some((o) => o.id === sel.id))
      this.selection.set(null);
    if (sel?.type === 'drawing' && !this.step().drawings.some((d) => d.id === sel.id))
      this.selection.set(null);
  }

  // --- small immutable helpers ---------------------------------------------

  private withStep(doc: TacticDoc, index: number, fn: (s: Step) => Step): TacticDoc {
    return { ...doc, steps: doc.steps.map((s, i) => (i === index ? fn(s) : s)) };
  }

  private withCurrentStep(fn: (s: Step) => Step): TacticDoc {
    return this.withStep(this.doc(), this.stepIndex(), fn);
  }

  // --- objects --------------------------------------------------------------

  place(at: Vec2): void {
    const what = this.placing();
    const doc = this.doc();
    const obj: BoardObject = { id: newId(), kind: what.kind };
    if (what.kind === 'player') {
      obj.team = what.team;
      if (what.keeper) obj.keeper = true;
      obj.number = what.keeper ? '1' : nextNumber(doc, what.team);
    } else if (what.kind !== 'ball') {
      obj.color = EQUIPMENT_COLORS[what.kind];
    }
    if (what.kind === 'goal' || what.kind === 'minigoal' || what.kind === 'popup') {
      // Face the centre of the pitch, which is nearly always what is wanted.
      obj.rotation = at[0] > 0 ? 0 : 180;
    }
    const next = this.withStep(
      { ...doc, objects: [...doc.objects, obj] },
      this.stepIndex(),
      (s) => ({
        ...s,
        pos: { ...s.pos, [obj.id]: at },
      }),
    );
    this.apply(next);
    this.selection.set({ type: 'object', id: obj.id });
  }

  moveObject(id: string, at: Vec2, done: boolean): void {
    const rounded: Vec2 = [Math.round(at[0] * 100) / 100, Math.round(at[1] * 100) / 100];
    this.apply(
      this.withCurrentStep((s) => ({ ...s, pos: { ...s.pos, [id]: rounded } })),
      done ? 'end' : 'move',
    );
  }

  /** Bends the path an object takes into this step through `mid`. */
  bend(id: string, mid: Vec2, done: boolean): void {
    const doc = this.doc();
    const i = this.stepIndex();
    const from = positionAt(doc, i - 1, id);
    const to = doc.steps[i]?.pos[id];
    if (!from || !to) return;
    const control = controlThrough(from, mid, to);
    this.apply(
      this.withCurrentStep((s) => ({ ...s, bend: { ...s.bend, [id]: control } })),
      done ? 'end' : 'move',
    );
  }

  straighten(id: string): void {
    this.apply(
      this.withCurrentStep((s) => {
        const bend = { ...s.bend };
        delete bend[id];
        return { ...s, bend };
      }),
    );
  }

  updateObject(id: string, patch: Partial<BoardObject>): void {
    const doc = this.doc();
    this.apply({
      ...doc,
      objects: doc.objects.map((o) => (o.id === id ? clean({ ...o, ...patch }) : o)),
    });
  }

  /** Toggles an id in one of the current step's lists: air or highlight. */
  toggleIn(list: 'air' | 'highlight', id: string): void {
    this.apply(
      this.withCurrentStep((s) => {
        const current = s[list] ?? [];
        return {
          ...s,
          [list]: current.includes(id) ? current.filter((x) => x !== id) : [...current, id],
        };
      }),
    );
  }

  duplicateSelection(): void {
    const obj = this.selectedObject();
    if (!obj) return;
    const at = positionAt(this.doc(), this.stepIndex(), obj.id);
    if (!at) return;
    const copy: BoardObject = { ...obj, id: newId() };
    if (copy.kind === 'player' && copy.team) copy.number = nextNumber(this.doc(), copy.team);
    const doc = this.doc();
    this.apply(
      this.withStep({ ...doc, objects: [...doc.objects, copy] }, this.stepIndex(), (s) => ({
        ...s,
        pos: { ...s.pos, [copy.id]: [at[0] + 3, at[1] + 2] },
      })),
    );
    this.selection.set({ type: 'object', id: copy.id });
  }

  deleteSelection(): void {
    const sel = this.selection();
    if (!sel) return;
    const doc = this.doc();
    if (sel.type === 'object') {
      const strip = <T>(rec: Record<string, T> | undefined) => {
        if (!rec) return rec;
        const out = { ...rec };
        delete out[sel.id];
        return out;
      };
      this.apply({
        ...doc,
        objects: doc.objects.filter((o) => o.id !== sel.id),
        steps: doc.steps.map((s) => ({
          ...s,
          pos: strip(s.pos)!,
          bend: strip(s.bend),
          air: s.air?.filter((x) => x !== sel.id),
          highlight: s.highlight?.filter((x) => x !== sel.id),
        })),
      });
    } else {
      this.apply(
        this.withCurrentStep((s) => ({
          ...s,
          drawings: s.drawings.filter((d) => d.id !== sel.id),
        })),
      );
    }
    this.selection.set(null);
  }

  addFormation(name: string, team: TeamId, side: 'left' | 'right'): void {
    const { objects, pos } = formation(name, team, side);
    const doc = this.doc();
    this.apply(
      this.withStep({ ...doc, objects: [...doc.objects, ...objects] }, this.stepIndex(), (s) => ({
        ...s,
        pos: { ...s.pos, ...pos },
      })),
    );
  }

  /** Removes every player of one team. */
  removeTeam(team: TeamId): void {
    const doc = this.doc();
    const gone = new Set(
      doc.objects.filter((o) => o.kind === 'player' && o.team === team).map((o) => o.id),
    );
    if (!gone.size) return;
    this.apply({
      ...doc,
      objects: doc.objects.filter((o) => !gone.has(o.id)),
      steps: doc.steps.map((s) => ({
        ...s,
        pos: Object.fromEntries(Object.entries(s.pos).filter(([id]) => !gone.has(id))),
      })),
    });
    this.selection.set(null);
  }

  // --- drawings -------------------------------------------------------------

  /** A stroke from a drawing tool: preview while drawing, commit on release. */
  stroke(points: Vec2[], done: boolean): void {
    const kind = this.tool() as DrawingKind;
    const pen = this.pen();
    const draft: Drawing = {
      id: 'preview',
      kind,
      points,
      color: pen.color,
      width: pen.width,
      style: kind === 'arrow' || kind === 'line' || kind === 'pen' ? pen.style : 'solid',
    };
    if (!done) {
      this.preview.set(points.length > 1 || kind === 'polygon' ? draft : null);
      return;
    }
    this.preview.set(null);

    const committed = {
      ...draft,
      id: newId(),
      points: kind === 'pen' ? thin(points, 0.25) : points,
    };
    // A click without a drag is not a drawing.
    const span = Math.hypot(
      points[points.length - 1][0] - points[0][0],
      points[points.length - 1][1] - points[0][1],
    );
    if (kind === 'polygon' ? points.length < 3 : span < 0.8) return;
    if (kind === 'rect' || kind === 'ellipse') committed.fill = 0.22;
    if (kind === 'polygon') committed.fill = 0.22;

    this.apply(this.withCurrentStep((s) => ({ ...s, drawings: [...s.drawings, committed] })));
    this.selection.set({ type: 'drawing', id: committed.id });
  }

  placeText(at: Vec2): void {
    const pen = this.pen();
    const d: Drawing = {
      id: newId(),
      kind: 'text',
      points: [at],
      color: pen.color,
      width: DEFAULT_STROKE,
      text: 'Text',
    };
    this.apply(this.withCurrentStep((s) => ({ ...s, drawings: [...s.drawings, d] })));
    this.selection.set({ type: 'drawing', id: d.id });
    this.tool.set('select');
    this.focusText.set(d.id);
  }

  editDrawing(id: string, change: DrawingChange, done: boolean): void {
    this.apply(
      this.withCurrentStep((s) => ({
        ...s,
        drawings: s.drawings.map((d) => {
          if (d.id !== id) return d;
          if ('offset' in change) {
            const [dx, dy] = change.offset;
            return {
              ...d,
              points: d.points.map(([x, y]) => [x + dx, y + dy] as Vec2),
              control: d.control ? [d.control[0] + dx, d.control[1] + dy] : undefined,
            };
          }
          if ('control' in change) {
            const a = d.points[0];
            const b = d.points[d.points.length - 1];
            return { ...d, control: controlThrough(a, change.control, b) };
          }
          return { ...d, points: d.points.map((p, i) => (i === change.index ? change.at : p)) };
        }),
      })),
      done ? 'end' : 'move',
    );
  }

  updateDrawing(id: string, patch: Partial<Drawing>): void {
    this.apply(
      this.withCurrentStep((s) => ({
        ...s,
        drawings: s.drawings.map((d) => (d.id === id ? clean({ ...d, ...patch }) : d)),
      })),
    );
  }

  // --- steps ----------------------------------------------------------------

  goToStep(i: number): void {
    this.stepIndex.set(Math.max(0, Math.min(this.doc().steps.length - 1, i)));
    if (this.selection()?.type === 'drawing') this.selection.set(null);
  }

  /** Adds a step after the current one. It starts as a copy of where things stand. */
  addStep(): void {
    const doc = this.doc();
    const at = this.stepIndex() + 1;
    const steps = [...doc.steps];
    steps.splice(at, 0, newStep());
    this.apply({ ...doc, steps });
    this.stepIndex.set(at);
    if (this.selection()?.type === 'drawing') this.selection.set(null);
  }

  /**
   * Inserts a copy of the current step right after it: same drawings, caption,
   * spotlights and timings. Nothing moves into the copy, since everything
   * already stands where the original left it; the move curves stay behind.
   */
  duplicateStep(): void {
    const doc = this.doc();
    const i = this.stepIndex();
    const { bend: _bend, air: _air, ...source } = doc.steps[i];
    const copy: Step = {
      ...source,
      id: newId(),
      pos: {},
      drawings: source.drawings.map((d) => ({ ...d, id: newId() })),
    };
    const steps = [...doc.steps];
    steps.splice(i + 1, 0, copy);
    this.apply({ ...doc, steps });
    this.stepIndex.set(i + 1);
    if (this.selection()?.type === 'drawing') this.selection.set(null);
  }

  /**
   * Moves a step to another place in the order and follows it there.
   *
   * A step stores only what moved in it, so reordering the raw steps would
   * change what every step around it shows. Instead each step is resolved to
   * where everything stands, the order is changed, and the positions are
   * stored again as differences from the new previous step. Every step then
   * looks the same as before; only the order they play in has changed.
   */
  moveStep(from: number, to: number): void {
    const doc = this.doc();
    const n = doc.steps.length;
    to = Math.max(0, Math.min(n - 1, to));
    if (from === to || from < 0 || from >= n) return;

    const resolved = doc.steps.map((_, i) => positionsAt(doc, i));
    const order = doc.steps.map((_, i) => i);
    order.splice(to, 0, ...order.splice(from, 1));

    const steps = order.map((at, k) => {
      const here = resolved[at];
      const before = k > 0 ? resolved[order[k - 1]] : new Map<string, Vec2>();
      const pos: Record<string, Vec2> = {};
      for (const [id, p] of here) {
        const q = before.get(id);
        if (!q || q[0] !== p[0] || q[1] !== p[1]) pos[id] = p;
      }
      return { ...doc.steps[at], pos };
    });
    this.apply({ ...doc, steps });
    this.stepIndex.set(to);
    if (this.selection()?.type === 'drawing') this.selection.set(null);
  }

  /**
   * Deletes the current step. Positions set in it are carried into the next
   * step, so everything after it stays where it was rather than jumping back.
   */
  deleteStep(): void {
    const doc = this.doc();
    if (doc.steps.length <= 1) return;
    const i = this.stepIndex();
    const removed = doc.steps[i];
    const steps = doc.steps.filter((_, n) => n !== i);
    if (i < steps.length) steps[i] = { ...steps[i], pos: { ...removed.pos, ...steps[i].pos } };
    this.apply({ ...doc, steps });
    this.stepIndex.set(Math.min(i, steps.length - 1));
    this.selection.set(null);
  }

  updateStep(patch: Partial<Step>): void {
    this.apply(this.withCurrentStep((s) => ({ ...s, ...patch })));
  }

  // --- board settings ---------------------------------------------------------

  setPitch(patch: Partial<PitchSettings>): void {
    const doc = this.doc();
    this.apply({ ...doc, pitch: { ...doc.pitch, ...patch } });
  }

  setKit(team: TeamId, patch: Partial<Kit>): void {
    const doc = this.doc();
    this.apply({ ...doc, teams: { ...doc.teams, [team]: { ...doc.teams[team], ...patch } } });
  }

  clearBoard(): void {
    const doc = this.doc();
    this.apply({ ...doc, objects: [], steps: [newStep()] });
    this.stepIndex.set(0);
    this.selection.set(null);
  }
}

/** Drops keys explicitly set to undefined, so they are not saved as nulls. */
function clean<T extends object>(o: T): T {
  for (const k of Object.keys(o) as (keyof T)[]) if (o[k] === undefined || o[k] === '') delete o[k];
  return o;
}
