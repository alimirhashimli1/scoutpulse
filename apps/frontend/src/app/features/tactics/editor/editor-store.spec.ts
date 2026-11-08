import { TestBed } from '@angular/core/testing';

import { positionAt } from '../board/model';
import { EditorStore } from './editor-store';

describe('EditorStore', () => {
  let store: EditorStore;

  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [EditorStore] });
    store = TestBed.inject(EditorStore);
  });

  function placePlayer(at: [number, number]): string {
    store.placing.set({ kind: 'player', team: 'home' });
    store.place(at);
    return store.selection()!.id;
  }

  it('places players without a number until one is given', () => {
    const id = placePlayer([0, 0]);
    placePlayer([5, 0]);
    expect(store.doc().objects.map((o) => o.number)).toEqual([undefined, undefined]);
    store.updateObject(id, { number: '9' });
    expect(store.doc().objects[0].number).toBe('9');
    store.updateObject(id, { number: '' });
    expect('number' in store.doc().objects[0]).toBe(false);
  });

  function drawArrow(): string {
    store.tool.set('arrow');
    store.stroke(
      [
        [0, 0],
        [10, 0],
      ],
      true,
    );
    return store.selection()!.id;
  }

  it('keeps a drawing in the steps after it until it is deleted', () => {
    const arrow = drawArrow();
    store.addStep();
    store.addStep();
    expect(store.doc().steps.map((s) => s.drawings.map((d) => d.id))).toEqual([
      [arrow],
      [arrow],
      [arrow],
    ]);

    store.goToStep(1);
    store.updateDrawing(arrow, { color: '#facc15' });
    expect(store.doc().steps.map((s) => s.drawings[0].color)).toEqual([
      '#ffffff',
      '#facc15',
      '#facc15',
    ]);

    store.selection.set({ type: 'drawing', id: arrow });
    store.deleteSelection();
    expect(store.doc().steps.map((s) => s.drawings.length)).toEqual([1, 0, 0]);
  });

  it('adds a new drawing to the steps that already follow', () => {
    store.addStep();
    store.goToStep(0);
    drawArrow();
    expect(store.doc().steps.map((s) => s.drawings.length)).toEqual([1, 1]);
  });

  it('records a whole drag as one undo step', () => {
    const id = placePlayer([0, 0]);
    store.moveObject(id, [1, 0], false);
    store.moveObject(id, [2, 0], false);
    store.moveObject(id, [3, 0], true);
    expect(positionAt(store.doc(), 0, id)).toEqual([3, 0]);
    store.undo();
    expect(positionAt(store.doc(), 0, id)).toEqual([0, 0]);
    store.redo();
    expect(positionAt(store.doc(), 0, id)).toEqual([3, 0]);
  });

  it('moves an object only from the current step onwards', () => {
    const id = placePlayer([0, 0]);
    store.addStep();
    store.moveObject(id, [10, 0], true);
    expect(positionAt(store.doc(), 0, id)).toEqual([0, 0]);
    expect(positionAt(store.doc(), 1, id)).toEqual([10, 0]);
  });

  it('keeps later steps in place when a step is deleted', () => {
    const id = placePlayer([0, 0]);
    store.addStep();
    store.moveObject(id, [10, 0], true);
    store.addStep();
    store.goToStep(1);
    store.deleteStep();
    expect(store.doc().steps).toHaveLength(2);
    expect(positionAt(store.doc(), 1, id)).toEqual([10, 0]);
  });

  it('duplicates a step after itself, carrying its drawings on', () => {
    const id = placePlayer([0, 0]);
    store.addStep();
    store.moveObject(id, [10, 0], true);
    store.updateStep({ note: 'Switch play', hold: 2 });
    store.tool.set('arrow');
    store.stroke(
      [
        [0, 0],
        [10, 0],
      ],
      true,
    );
    store.addStep();
    store.goToStep(1);

    store.duplicateStep();
    const [, original, copy] = store.doc().steps;
    expect(store.doc().steps).toHaveLength(4);
    expect(store.stepIndex()).toBe(2);
    expect(copy).toMatchObject({ note: 'Switch play', hold: 2, pos: {} });
    expect(copy.drawings).toHaveLength(1);
    expect(copy.drawings[0].id).toBe(original.drawings[0].id);
    expect(copy.id).not.toBe(original.id);
    expect(positionAt(store.doc(), 2, id)).toEqual([10, 0]);
    store.undo();
    expect(store.doc().steps).toHaveLength(3);
  });

  it('reorders steps without changing what each one shows', () => {
    const a = placePlayer([0, 0]);
    const b = placePlayer([5, 0]);
    store.addStep();
    store.moveObject(a, [10, 0], true);
    store.updateStep({ note: 'A runs' });
    store.addStep();
    store.moveObject(b, [5, 8], true);
    store.updateStep({ note: 'B drops' });
    const at = (i: number) => [positionAt(store.doc(), i, a), positionAt(store.doc(), i, b)];
    const before = [0, 1, 2].map(at);

    store.moveStep(2, 1);
    expect(store.doc().steps.map((s) => s.note)).toEqual(['', 'B drops', 'A runs']);
    expect(store.stepIndex()).toBe(1);
    expect([0, 2, 1].map(at)).toEqual(before);

    store.moveStep(1, 0);
    expect(store.doc().steps.map((s) => s.note)).toEqual(['B drops', '', 'A runs']);
    expect(at(0)).toEqual(before[2]);
    store.undo();
    store.undo();
    expect(store.doc().steps.map((s) => s.note)).toEqual(['', 'A runs', 'B drops']);
  });

  it('removes a deleted object from every step', () => {
    const id = placePlayer([0, 0]);
    store.addStep();
    store.moveObject(id, [4, 4], true);
    store.toggleIn('highlight', id);
    store.selection.set({ type: 'object', id });
    store.deleteSelection();
    expect(store.doc().objects).toHaveLength(0);
    expect(store.doc().steps.every((s) => !(id in s.pos) && !s.highlight?.includes(id))).toBe(true);
  });

  it('ignores a click with a drawing tool, and commits a drag', () => {
    store.tool.set('arrow');
    store.stroke(
      [
        [0, 0],
        [0.1, 0],
      ],
      true,
    );
    expect(store.step().drawings).toHaveLength(0);
    store.stroke(
      [
        [0, 0],
        [10, 0],
      ],
      true,
    );
    expect(store.step().drawings).toHaveLength(1);
    expect(store.step().drawings[0].kind).toBe('arrow');
  });

  it('marks the board dirty on change and clean on save', () => {
    expect(store.dirty()).toBe(false);
    placePlayer([0, 0]);
    expect(store.dirty()).toBe(true);
    store.markSaved();
    expect(store.dirty()).toBe(false);
  });
});
