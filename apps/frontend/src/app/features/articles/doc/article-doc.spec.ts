import {
  ArticleDoc,
  isEmptyDoc,
  normaliseLink,
  outline,
  parseTacticSlug,
  parseVideo,
  safeImage,
  safeLink,
  videoEmbedUrl,
  wordCount,
} from './article-doc';

describe('article addresses', () => {
  it('lets through only links a reader may safely follow', () => {
    for (const ok of [
      'https://www.uefa.com/x',
      'http://example.com',
      'mailto:desk@example.com',
      '/players/123',
    ]) {
      expect(safeLink(ok)).toBe(ok);
    }
    for (const bad of [
      'javascript:alert(1)',
      'JAVASCRIPT:alert(1)',
      'data:text/html,hi',
      '//evil.example.com',
      '/\\evil.example.com',
      'https://exa mple.com',
      '',
      null,
      42,
    ]) {
      expect(safeLink(bad)).toBeNull();
    }
  });

  it('loads images only over https', () => {
    expect(safeImage('https://img.example.com/a.jpg')).toBe('https://img.example.com/a.jpg');
    expect(safeImage('http://img.example.com/a.jpg')).toBeNull();
    expect(safeImage('data:image/png;base64,AAAA')).toBeNull();
    expect(safeImage('/local.png')).toBeNull();
  });

  it('makes a link out of what a writer is likely to paste', () => {
    expect(normaliseLink('www.uefa.com/news')).toBe('https://www.uefa.com/news');
    expect(normaliseLink('desk@example.com')).toBe('mailto:desk@example.com');
    expect(normaliseLink('/clubs/1')).toBe('/clubs/1');
    expect(normaliseLink('javascript:alert(1)')).toBeNull();
    expect(normaliseLink('   ')).toBeNull();
  });

  it('reads every shape of video link the share buttons produce', () => {
    const id = 'dQw4w9WgXcQ';
    for (const url of [
      `https://www.youtube.com/watch?v=${id}&t=42`,
      `https://youtu.be/${id}?si=abc`,
      `https://m.youtube.com/watch?v=${id}`,
      `https://www.youtube.com/shorts/${id}`,
      `https://www.youtube.com/embed/${id}`,
    ]) {
      expect(parseVideo(url)).toEqual({ provider: 'youtube', id });
    }
    expect(parseVideo('https://vimeo.com/76979871')).toEqual({ provider: 'vimeo', id: '76979871' });
    expect(parseVideo('https://example.com/watch?v=dQw4w9WgXcQ')).toBeNull();
    expect(parseVideo('not a url')).toBeNull();
    expect(videoEmbedUrl('youtube', id)).toContain('youtube-nocookie.com/embed/' + id);
    expect(videoEmbedUrl('youtube', '"><script>')).toBeNull();
  });

  it('finds a board in its share link, its edit link, or the bare slug', () => {
    const slug = 'aB3dE5fG7hJ9';
    expect(parseTacticSlug(slug)).toBe(slug);
    expect(parseTacticSlug(`https://scoutpulse.app/tactics/${slug}`)).toBe(slug);
    expect(parseTacticSlug(`/tactics/${slug}/edit`)).toBe(slug);
    // The board alphabet has no 0, 1, l, o, I or O.
    expect(parseTacticSlug('aB3dE5fG7hJ0')).toBeNull();
    expect(parseTacticSlug('abcdefghijlo')).toBeNull();
    expect(parseTacticSlug('/tactics/../../etc')).toBeNull();
  });
});

describe('reading the document', () => {
  const doc: ArticleDoc = {
    type: 'doc',
    content: [
      { type: 'heading', attrs: { level: 2 }, content: [{ type: 'text', text: 'The press' }] },
      { type: 'paragraph', content: [{ type: 'text', text: 'Arsenal pressed high early.' }] },
      {
        type: 'heading',
        attrs: { level: 2 },
        content: [{ type: 'text', text: 'Atlético’s block' }],
      },
      { type: 'heading', attrs: { level: 3 }, content: [{ type: 'text', text: 'The press' }] },
    ],
  };

  it('gives every heading a stable, unique anchor', () => {
    const { entries } = outline(doc);
    expect(entries.map((e) => e.id)).toEqual(['the-press', 'atletico-s-block', 'the-press-2']);
    expect(entries.map((e) => e.level)).toEqual([2, 2, 3]);
  });

  it('counts words, and knows an empty document from one with only an embed', () => {
    expect(wordCount(doc)).toBe(10);
    expect(isEmptyDoc({ type: 'doc', content: [{ type: 'paragraph' }] })).toBe(true);
    expect(
      isEmptyDoc({
        type: 'doc',
        content: [{ type: 'video', attrs: { provider: 'youtube', id: 'x' } }],
      }),
    ).toBe(false);
  });
});
