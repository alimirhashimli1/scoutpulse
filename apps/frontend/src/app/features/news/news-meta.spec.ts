import { importance, isCategory, timeAgo } from './news-meta';

describe('news meta', () => {
  const now = new Date('2026-11-10T12:00:00Z');

  it('formats relative time', () => {
    expect(timeAgo('2026-11-10T11:59:30Z', now)).toBe('just now');
    expect(timeAgo('2026-11-10T11:55:00Z', now)).toBe('5m ago');
    expect(timeAgo('2026-11-10T10:00:00Z', now)).toBe('2h ago');
    expect(timeAgo('2026-11-07T12:00:00Z', now)).toBe('3d ago');
    expect(timeAgo('2026-10-01T12:00:00Z', now)).toBe('1 Oct');
    expect(timeAgo('not a date', now)).toBe('');
  });

  it('names the importance band for assistive technology', () => {
    expect(importance(9)).toEqual({ level: 'major', label: 'Major story, importance 9 of 10' });
    expect(importance(7).level).toBe('high');
  });

  it('accepts only the six categories', () => {
    expect(isCategory('young_talents')).toBe(true);
    expect(isCategory('gossip')).toBe(false);
    expect(isCategory(undefined)).toBe(false);
  });
});
