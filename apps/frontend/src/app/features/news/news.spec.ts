import { Type } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { NEWS_READER, NewsReader } from '../../core/api/news';
import { TRANSFER_READER } from '../../core/api/contracts';
import { LookupStore } from '../../core/api/lookup-store';
import { PlayerNames } from '../../core/api/player-names';
import { NewsStory } from '../../core/models/news';
import { SITE_URL } from '../../core/tokens/site-url';
import { TransferFeed } from '../transfers/transfer-feed';
import { NewsPage } from './news-page';
import { TopHeadlines } from './top-headlines';

const story = (id: string, over: Partial<NewsStory> = {}): NewsStory => ({
  id,
  headline: `Headline ${id}`,
  url: `https://bbc.co.uk/sport/${id}`,
  source_name: 'BBC Sport',
  attribution: 'Headline and link from BBC Sport',
  summary: `Summary ${id}`,
  category: 'transfers',
  score: 8,
  source_count: 1,
  published_at: new Date(Date.now() - 2 * 3600_000).toISOString(),
  pinned: false,
  ...over,
});

function reader(over: Partial<NewsReader> = {}): NewsReader {
  return {
    list: () => Promise.resolve({ items: [], limit: 20, offset: 0, has_more: false }),
    top: () => Promise.resolve([]),
    ...over,
  };
}

function configure(news: NewsReader) {
  TestBed.configureTestingModule({
    providers: [
      provideRouter([]),
      { provide: NEWS_READER, useValue: news },
      { provide: SITE_URL, useValue: 'http://test.local' },
      {
        provide: TRANSFER_READER,
        useValue: {
          list: () => Promise.resolve({ items: [], limit: 25, offset: 0, has_more: false }),
        },
      },
      { provide: LookupStore, useValue: { loadTeams: async () => {}, teamName: () => '—' } },
      { provide: PlayerNames, useValue: { resolve: async () => {}, name: () => '' } },
    ],
  });
}

async function render<T>(cmp: Type<T>, inputs: Record<string, unknown> = {}) {
  const fixture = TestBed.createComponent(cmp);
  for (const [k, v] of Object.entries(inputs)) fixture.componentRef.setInput(k, v);
  await fixture.whenStable();
  fixture.detectChanges();
  await fixture.whenStable();
  return fixture.nativeElement as HTMLElement;
}

describe('TopHeadlines', () => {
  it('shows compact cards and a link to the news page', async () => {
    configure(reader({ top: () => Promise.resolve([story('a'), story('b'), story('c')]) }));
    const root = await render(TopHeadlines);
    expect(root.querySelector('h2')?.textContent).toContain('Top Headlines');
    expect(root.querySelectorAll('app-news-card').length).toBe(3);
    expect(root.querySelector('a[href="/news"]')?.textContent).toContain('See all news');
    const out = root.querySelector<HTMLAnchorElement>('.headline a')!;
    expect(out.target).toBe('_blank');
    expect(out.rel).toContain('noopener');
  });

  it('renders nothing when there are no stories', async () => {
    configure(reader());
    const root = await render(TopHeadlines);
    expect(root.querySelector('section')).toBeNull();
  });

  it('renders nothing when the request fails', async () => {
    configure(reader({ top: () => Promise.reject(new Error('down')) }));
    const root = await render(TopHeadlines);
    expect(root.querySelector('section')).toBeNull();
  });
});

describe('home page', () => {
  it('carries the top headlines on the home route only', async () => {
    configure(reader({ top: () => Promise.resolve([story('a')]) }));
    const home = await render(TransferFeed, { home: true });
    expect(home.querySelector('app-top-headlines section')).toBeTruthy();

    const transfers = await render(TransferFeed);
    expect(transfers.querySelector('app-top-headlines')).toBeNull();
  });
});

describe('NewsPage', () => {
  it('lists stories with attribution, time, category and a read-at link', async () => {
    configure(
      reader({
        list: () =>
          Promise.resolve({
            items: [story('a', { score: 9 }), story('b')],
            limit: 20,
            offset: 0,
            has_more: true,
          }),
      }),
    );
    const root = await render(NewsPage);
    const cards = root.querySelectorAll<HTMLElement>('app-news-card');
    expect(cards.length).toBe(2);
    const first = cards[0];
    expect(first.querySelector('.badge')?.textContent).toContain('Transfers');
    expect(first.querySelector('.summary')?.textContent).toContain('Summary a');
    expect(first.querySelector('time')?.textContent).toContain('2h ago');
    expect(first.querySelector('[role="img"]')?.getAttribute('aria-label')).toContain(
      'importance 9 of 10',
    );
    expect(first.querySelector('.read')?.textContent).toContain('Read at BBC Sport');
    expect(first.querySelector('img')).toBeNull();
    expect(root.querySelector('button.more')?.textContent).toContain('Load more');
    const filters = Array.from(root.querySelectorAll('.filters a')).map((a) =>
      a.textContent?.trim(),
    );
    expect(filters).toEqual([
      'All',
      'Transfers',
      'Injuries',
      'Results',
      'Managers',
      'Young Talents',
      'Other',
    ]);
  });

  it('passes the category filter to the API', async () => {
    const calls: unknown[] = [];
    configure(
      reader({
        list: (f) => {
          calls.push(f);
          return Promise.resolve({ items: [], limit: 20, offset: 0, has_more: false });
        },
      }),
    );
    const root = await render(NewsPage, { category: 'injuries' });
    expect(calls[0]).toEqual({ category: 'injuries', limit: 20 });
    expect(root.querySelector('app-empty')?.textContent).toContain('No Injuries stories');
    expect(root.querySelector('.filters a.active')?.textContent?.trim()).toBe('Injuries');
  });

  it('shows an error state when the feed cannot load', async () => {
    configure(reader({ list: () => Promise.reject(new Error('down')) }));
    const root = await render(NewsPage);
    expect(root.querySelector('app-error-state')).toBeTruthy();
  });
});
