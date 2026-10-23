import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { ArticleDoc } from './article-doc';
import { ArticleBody } from './article-body';

async function render(doc: ArticleDoc): Promise<HTMLElement> {
  TestBed.configureTestingModule({ imports: [ArticleBody], providers: [provideRouter([])] });
  // The board embed brings an external stylesheet, which has to be compiled first.
  await TestBed.compileComponents();
  const fixture = TestBed.createComponent(ArticleBody);
  fixture.componentRef.setInput('doc', doc);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

const para = (...content: ArticleDoc[]): ArticleDoc => ({ type: 'paragraph', content });
const text = (t: string, marks?: ArticleDoc['marks']): ArticleDoc => ({
  type: 'text',
  text: t,
  marks,
});

describe('ArticleBody', () => {
  it('draws marks without adding spaces around them', async () => {
    const el = await render({
      type: 'doc',
      content: [
        para(text('Saka '), text('cuts inside', [{ type: 'bold' }, { type: 'italic' }]), text('.')),
      ],
    });
    const p = el.querySelector('p')!;
    expect(p.textContent).toBe('Saka cuts inside.');
    expect(p.querySelector('strong em')?.textContent).toBe('cuts inside');
  });

  it('keeps pasted markup as text', async () => {
    const el = await render({ type: 'doc', content: [para(text('<img src=x onerror=alert(1)>'))] });
    expect(el.querySelector('img')).toBeNull();
    expect(el.querySelector('p')!.textContent).toBe('<img src=x onerror=alert(1)>');
  });

  it('drops a link it would not follow, but keeps its text', async () => {
    const el = await render({
      type: 'doc',
      content: [
        para(
          text('bad', [{ type: 'link', attrs: { href: 'javascript:alert(1)' } }]),
          text(' '),
          text('good', [{ type: 'link', attrs: { href: 'https://www.uefa.com' } }]),
          text(' '),
          text('here', [{ type: 'link', attrs: { href: '/players/1' } }]),
        ),
      ],
    });
    const links = [...el.querySelectorAll('a')];
    expect(links.map((a) => a.getAttribute('href'))).toEqual([
      'https://www.uefa.com',
      '/players/1',
    ]);
    expect(links[0].getAttribute('target')).toBe('_blank');
    expect(links[0].getAttribute('rel')).toContain('noopener');
    expect(links[1].getAttribute('target')).toBeNull();
    expect(el.textContent).toContain('bad good here');
  });

  it('skips unknown blocks and unsafe images', async () => {
    const el = await render({
      type: 'doc',
      content: [
        { type: 'iframe', attrs: { src: 'https://evil.example.com' } },
        { type: 'figure', attrs: { src: 'javascript:alert(1)' } },
        {
          type: 'figure',
          attrs: { src: 'https://img.example.com/a.jpg', alt: 'A press', caption: 'Pressing' },
        },
      ],
    });
    expect(el.querySelector('iframe')).toBeNull();
    const imgs = el.querySelectorAll('img');
    expect(imgs).toHaveLength(1);
    expect(imgs[0].getAttribute('src')).toBe('https://img.example.com/a.jpg');
    expect(imgs[0].getAttribute('referrerpolicy')).toBe('no-referrer');
    expect(el.querySelector('figcaption')!.textContent).toContain('Pressing');
  });

  it('anchors headings for the contents list', async () => {
    const el = await render({
      type: 'doc',
      content: [{ type: 'heading', attrs: { level: 2 }, content: [text('The press')] }],
    });
    expect(el.querySelector('h2')!.id).toBe('the-press');
  });
});
