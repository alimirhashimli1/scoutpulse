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

  it('numbers players as they are placed', () => {
    placePlayer([0, 0]);
    placePlayer([5, 0]);
    expect(store.doc().objects.map((o) => o.number)).toEqual(['1', '2']);
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

  it('duplicates a step after itself, with its own copies of the drawings', () => {
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
    expect(copy.drawings[0].id).not.toBe(original.drawings[0].id);
    expect(copy.id).not.toBe(original.id);
    expect(positionAt(store.doc(), 2, id)).toEqual([10, 0]);
    store.undo();
    expect(store.doc().steps).toHaveLength(3);
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
