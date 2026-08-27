import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { Type } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { API_CONFIG, apiConfigFor } from '../tokens/api-config';
import { SITE_URL } from '../tokens/site-url';
import { provideFootballApi } from './providers';

import { ClubList } from '../../features/clubs/club-list';
import { CompetitionList } from '../../features/competitions/competition-pages';
import { SeasonList } from '../../features/seasons/season-pages';
import { TransferFeed } from '../../features/transfers/transfer-feed';
import { PlayerPage } from '../../features/players/player-page';
import { ClubPage } from '../../features/clubs/club-page';
import { CoachPage } from '../../features/coaches/coach-page';

/**
 * Every page that seeds its resource from the server render has to survive
 * being constructed in a browser, which sounds obvious and was not.
 *
 * `resource()` treats `params` as a reactive computation and reads it lazily.
 * An earlier version of ssrResource called it during field initialization to
 * build a cache key from the params — which reads fields declared further down
 * the class, and required inputs Angular has not set yet, and throws on both.
 *
 * The server never ran that path, so SSR kept working and every client-side
 * navigation rendered an empty page: header, footer, and nothing between. The
 * assertions below are deliberately shallow. They are not about what these
 * pages show; they are about whether they come up at all.
 */
describe('components seeded by ssrResource', () => {
  beforeEach(() =>
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        provideHttpClient(),
        provideHttpClientTesting(),
        { provide: API_CONFIG, useValue: apiConfigFor('http://test.local') },
        { provide: SITE_URL, useValue: 'http://test.local' },
        ...provideFootballApi(),
      ],
    }),
  );

  describe('list pages construct and render', () => {
    const lists: [string, Type<object>][] = [
      ['clubs', ClubList],
      ['competitions', CompetitionList],
      ['seasons', SeasonList],
      ['transfers', TransferFeed],
    ];

    for (const [name, type] of lists) {
      it(name, () => {
        const fixture = TestBed.createComponent(type);
        fixture.detectChanges();
        expect((fixture.nativeElement as HTMLElement).querySelector('main')).toBeTruthy();
      });
    }
  });

  describe('detail pages construct with their route input set', () => {
    const details: [string, Type<object>][] = [
      ['player', PlayerPage],
      ['club', ClubPage],
      ['coach', CoachPage],
    ];

    for (const [name, type] of details) {
      it(name, () => {
        const fixture = TestBed.createComponent(type);
        // The id arrives as a component input from the router. Setting it after
        // construction is exactly the ordering that broke: anything reading it
        // during field initialization has already thrown by now.
        fixture.componentRef.setInput('id', '00000000-0000-0000-0000-000000000000');
        fixture.detectChanges();
        expect((fixture.nativeElement as HTMLElement).querySelector('main')).toBeTruthy();
      });
    }
  });
});
