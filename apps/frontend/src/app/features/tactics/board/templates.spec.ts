import { normalizeDoc, positionsAt } from './model';
import { TEMPLATES } from './templates';

describe('board templates', () => {
  for (const [key, t] of Object.entries(TEMPLATES)) {
    it(`${key} is a complete board`, () => {
      const doc = t.doc();
      expect(normalizeDoc(doc)).toEqual(doc);
      const ids = new Set(doc.objects.map((o) => o.id));
      // Every object stands somewhere from the first step on.
      expect(positionsAt(doc, 0).size).toBe(ids.size);
      for (const s of doc.steps) {
        expect(Object.keys(s.pos).every((id) => ids.has(id))).toBe(true);
        expect(s.note.length).toBeGreaterThan(0);
      }
    });
  }

  it('pressing drill 1 has its ten steps', () => {
    expect(TEMPLATES['pressing-drill-1'].doc().steps).toHaveLength(10);
  });
});
