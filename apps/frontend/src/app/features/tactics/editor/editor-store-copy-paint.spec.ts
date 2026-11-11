import { emptyDoc, positionAt, readableOn, HALF_LENGTH, HALF_WIDTH, RUNOFF } from '../board/model';
import { EditorStore } from './editor-store';

function storeWith(x = 0, y = 0): EditorStore {
  const store = new EditorStore();
  const doc = emptyDoc();
  doc.objects = [{ id: 'p', kind: 'player', team: 'home', number: '7', name: 'Ali' }];
  doc.steps[0].pos = { p: [x, y] };
  store.load(doc);
  store.selection.set({ type: 'object', id: 'p' });
  return store;
}

describe('copy and paste', () => {
  it('pastes a piece beside the original and selects it', () => {
    const store = storeWith();
    expect(store.copySelection()).toBe(true);
    expect(store.paste()).toBe(true);

    const objects = store.doc().objects;
    expect(objects).toHaveLength(2);
    const copy = objects[1];
    expect(copy.id).not.toBe('p');
    expect(copy.name).toBe('Ali');
    expect(copy.number).toBeUndefined();
    expect(positionAt(store.doc(), 0, copy.id)).toEqual([3, 2]);
    expect(store.selection()).toEqual({ type: 'object', id: copy.id });
  });

  it('spreads repeated pastes out rather than stacking them', () => {
    const store = storeWith();
    store.copySelection();
    store.paste();
    store.paste();
    const [, a, b] = store.doc().objects;
    expect(positionAt(store.doc(), 0, a.id)).toEqual([3, 2]);
    expect(positionAt(store.doc(), 0, b.id)).toEqual([6, 4]);
  });

  it('pastes back towards the pitch from the very edge', () => {
    const store = storeWith(HALF_LENGTH + RUNOFF, HALF_WIDTH + RUNOFF);
    store.copySelection();
    store.paste();
    expect(positionAt(store.doc(), 0, store.doc().objects[1].id)).toEqual([
      HALF_LENGTH + RUNOFF - 3,
      HALF_WIDTH + RUNOFF - 2,
    ]);
  });

  it('still pastes after the original is deleted, and undoes in one go', () => {
    const store = storeWith();
    store.copySelection();
    store.deleteSelection();
    expect(store.doc().objects).toHaveLength(0);
    store.paste();
    expect(store.doc().objects).toHaveLength(1);
    store.undo();
    expect(store.doc().objects).toHaveLength(0);
  });

  it('copies a drawing with its curve, shifted, into this step onward', () => {
    const store = storeWith();
    store.tool.set('arrow');
    store.stroke(
      [
        [0, 0],
        [10, 0],
      ],
      true,
    );
    const id = store.step().drawings[0].id;
    store.updateDrawing(id, { control: [5, 5] });
    store.selection.set({ type: 'drawing', id });
    store.copySelection();
    store.paste();

    const [, copy] = store.step().drawings;
    expect(copy.id).not.toBe(id);
    expect(copy.points).toEqual([
      [3, 2],
      [13, 2],
    ]);
    expect(copy.control).toEqual([8, 7]);
  });

  it('has nothing to do without a selection or a copy', () => {
    const store = storeWith();
    expect(store.paste()).toBe(false);
    store.selection.set(null);
    expect(store.copySelection()).toBe(false);
  });
});

describe('painting a team', () => {
  function team(): EditorStore {
    const store = new EditorStore();
    const doc = emptyDoc();
    doc.objects = [
      { id: 'a', kind: 'player', team: 'home', color: '#111111', pattern: 'hoops', bib: '#ff0000' },
      { id: 'k', kind: 'player', team: 'home', keeper: true, color: '#222222' },
      { id: 'b', kind: 'player', team: 'away', color: '#333333' },
      { id: 'c', kind: 'cone', color: '#444444' },
    ];
    store.load(doc);
    return store;
  }

  it('puts every outfield player of the team in one colour', () => {
    const store = team();
    store.paintTeam('home', '#ffffff', false);
    const doc = store.doc();
    expect(doc.teams.home).toMatchObject({
      primary: '#ffffff',
      pattern: 'solid',
      numbers: '#111827',
    });
    const [a, k, b, c] = doc.objects;
    expect(a).toEqual({ id: 'a', kind: 'player', team: 'home' });
    expect(k.color).toBe('#222222');
    expect(b.color).toBe('#333333');
    expect(c.color).toBe('#444444');
  });

  it('includes goalkeepers only when asked', () => {
    const store = team();
    const keeperBefore = store.doc().teams.home.keeper;
    store.paintTeam('home', '#123456', false);
    expect(store.doc().teams.home.keeper).toBe(keeperBefore);
    store.paintTeam('home', '#123456', true);
    expect(store.doc().teams.home.keeper).toBe('#123456');
    expect(store.doc().objects[1].color).toBeUndefined();
  });

  it('picks numbers that can be read on the shirt', () => {
    expect(readableOn('#ffffff')).toBe('#111827');
    expect(readableOn('#111827')).toBe('#ffffff');
  });
});
