import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  resource,
  signal,
} from '@angular/core';
import { RouterLink } from '@angular/router';

import { ApiError } from '../../core/api/api-error';
import { LEAGUE_READER, SEASON_READER, TEAM_READER } from '../../core/api/contracts';
import { LookupStore } from '../../core/api/lookup-store';
import { PageQuery } from '../../core/api/page';
import { Permissions } from '../../core/auth/permissions';
import { Seo } from '../../core/seo/seo';
import { messageFor } from '../../shared/forms/submit';
import { Paginator } from '../../shared/pagination/paginator';
import { Empty, ErrorState, Loading } from '../../shared/ui/states';

const TYPE_LABELS: Record<string, string> = {
  league: 'League',
  domestic_cup: 'Domestic cup',
  international_cup: 'International cup',
  super_cup: 'Super cup',
};

@Component({
  selector: 'app-competition-list',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, Paginator, Loading, Empty, ErrorState],
  template: `
    <main class="page">
      <header class="masthead">
        <div>
          <h1>Competitions</h1>
          <p class="standfirst">Leagues and cups on record.</p>
        </div>
        @if (permissions.canAdminister()) {
          <a class="btn primary" routerLink="/competitions/new">New competition</a>
        }
      </header>

      @if (leagues.isLoading()) {
        <app-loading message="Loading competitions…" />
      } @else if (leagues.error()) {
        <app-error-state [message]="errorMessage()" />
      } @else if (!leagues.value()?.items?.length) {
        <app-empty message="No competitions yet." />
      } @else {
        <ul class="list">
          @for (league of leagues.value()!.items; track league.id) {
            <li>
              <a [routerLink]="['/competitions', league.id]">
                <span class="name">{{ league.name }}</span>
                <span class="meta">
                  {{ league.country }}
                  <span class="muted">· {{ typeLabel(league.competition_type) }}</span>
                  @if (league.tier) {
                    <span class="muted">· tier {{ league.tier }}</span>
                  }
                </span>
              </a>
            </li>
          }
        </ul>
        <app-paginator
          [page]="leagues.value()!"
          label="competition results"
          (pageChange)="goTo($event)"
        />
      }
    </main>
  `,
  styles: `
    .masthead {
      display: flex;
      justify-content: space-between;
      align-items: flex-end;
      gap: var(--space-4);
      flex-wrap: wrap;
      padding-block: var(--space-6) var(--space-5);
    }
    h1 {
      margin-bottom: var(--space-2);
    }
    .standfirst {
      color: var(--ink-soft);
    }
    .list {
      list-style: none;
      margin: 0;
      padding: 0;
    }
    .list li {
      border-bottom: 1px solid var(--line-soft);
    }
    .list a {
      display: flex;
      justify-content: space-between;
      gap: var(--space-4);
      align-items: baseline;
      padding: var(--space-3) 0;
      text-decoration: none;
      color: var(--ink);
    }
    .list a:hover .name {
      color: var(--accent);
    }
    .name {
      font-weight: 600;
    }
    .meta {
      color: var(--ink-soft);
      font-size: var(--text-sm);
    }
    .muted {
      color: var(--muted);
    }
  `,
})
export class CompetitionList {
  private readonly reader = inject(LEAGUE_READER);
  protected readonly permissions = inject(Permissions);
  private readonly seo = inject(Seo);
  private readonly page = signal<PageQuery>({ limit: 25, offset: 0 });

  constructor() {
    this.seo.describe({
      title: 'Competitions',
      description: 'Every league and cup on record, by country and tier.',
      path: '/competitions',
    });
  }

  protected readonly leagues = resource({
    params: () => ({ page: this.page() }),
    loader: ({ params }) => this.reader.list(params.page),
  });

  protected readonly errorMessage = computed(() => {
    const error = this.leagues.error();
    return error instanceof Error ? error.message : 'Could not load competitions.';
  });

  protected typeLabel(type: string): string {
    return TYPE_LABELS[type] ?? type;
  }

  protected goTo(query: PageQuery): void {
    this.page.set(query);
  }
}

/**
 * A competition, and who is in it — either now or in a chosen season.
 *
 * The two are genuinely different questions, which is why the picker exists
 * rather than one list pretending to answer both:
 *
 * - **Now** reads `teams.league_id`, a single pointer to a club's present
 *   competition. Relegation overwrites it, so it says nothing about the past.
 * - **A season** reads team_seasons, the record of who actually contested the
 *   competition that year. It survives relegation because it is history.
 *
 * They can legitimately disagree, and neither is wrong.
 *
 * An earlier version of this comment said a season view "belongs with a season
 * picker, not here" — and then no such page was ever built, so the entries the
 * club page writes had nowhere to be read. This is that page.
 *
 * There is still no table. There is no match data, so there is nothing to
 * compute a standing from, and a placeholder implying otherwise would be a lie.
 */
@Component({
  selector: 'app-competition-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, Paginator, Loading, Empty, ErrorState],
  template: `
    @if (league.isLoading()) {
      <main class="page"><app-loading message="Loading competition…" /></main>
    } @else if (league.error()) {
      <main class="page"><app-error-state [message]="errorMessage()" /></main>
    } @else if (league.value(); as l) {
      <main class="page">
        <header class="head">
          <p class="eyebrow">{{ typeLabel(l.competition_type) }}</p>
          <h1>{{ l.name }}</h1>
          <p class="standfirst">
            {{ l.country }}
            @if (l.tier) {
              <span> · tier {{ l.tier }}</span>
            }
          </p>
          @if (permissions.canAdminister()) {
            <p class="edit">
              <a class="btn" [routerLink]="['/competitions', l.id, 'edit']">Edit</a>
            </p>
          }
        </header>

        <section>
          <div class="section-head">
            <h4>Clubs</h4>

            <!--
              "Now" is not a season, and that is the point. The default list is
              teams.league_id — where clubs are *currently* — which is a
              single pointer relegation overwrites. Picking a season switches to
              team_seasons, the record of who actually contested it that year.
              The two answer different questions and can legitimately disagree.
            -->
            @if (seasons().length) {
              <label class="picker">
                <span class="visually-hidden">Season</span>
                <select [value]="seasonId()" (change)="pickSeason($event)">
                  <option value="">Now</option>
                  @for (season of seasons(); track season.id) {
                    <option [value]="season.id">{{ season.label }}</option>
                  }
                </select>
              </label>
            }
          </div>

          @if (seasonId()) {
            @if (entrants.isLoading()) {
              <app-loading [lines]="3" />
            } @else if (entrants.error()) {
              <app-error-state [message]="entrantsErrorMessage()" />
            } @else if (!entrants.value()?.items?.length) {
              <app-empty
                [message]="'No clubs recorded in this competition for ' + seasonLabel() + '.'"
                hint="Entries are recorded per club, from the club's own page."
              />
            } @else {
              <ul class="list">
                @for (entry of entrants.value()!.items; track entry.id) {
                  <li>
                    <a [routerLink]="['/clubs', entry.team_id]">
                      <span class="name">{{ clubName(entry.team_id) }}</span>
                      <span class="meta">{{ seasonLabel() }}</span>
                    </a>
                  </li>
                }
              </ul>
            }
          } @else if (clubs.isLoading()) {
            <app-loading />
          } @else if (!clubs.value()?.items?.length) {
            <app-empty message="No clubs in this competition yet." />
          } @else {
            <ul class="list">
              @for (club of clubs.value()!.items; track club.id) {
                <li>
                  <a [routerLink]="['/clubs', club.id]">
                    <span class="name">{{ club.name }}</span>
                    <span class="meta">{{ club.city ?? '' }}</span>
                  </a>
                </li>
              }
            </ul>
            <app-paginator [page]="clubs.value()!" label="clubs" (pageChange)="goTo($event)" />
          }
        </section>
      </main>
    }
  `,
  styles: `
    .head {
      border-bottom: 1px solid var(--line);
      padding-block: var(--space-6) var(--space-5);
      margin-bottom: var(--space-6);
    }
    .eyebrow {
      font-family: var(--font-mono);
      font-size: var(--text-xs);
      letter-spacing: 0.12em;
      text-transform: uppercase;
      color: var(--muted);
      margin-bottom: var(--space-2);
    }
    h1 {
      font-size: var(--text-3xl);
      margin-bottom: var(--space-2);
    }
    .standfirst {
      color: var(--ink-soft);
    }
    .edit {
      margin-top: var(--space-4);
    }
    .section-head {
      display: flex;
      justify-content: space-between;
      align-items: baseline;
      gap: var(--space-4);
      margin-bottom: var(--space-3);
    }
    .section-head h4 {
      margin-bottom: 0;
    }
    .picker select {
      padding: var(--space-1) var(--space-2);
      border: 1px solid var(--line);
      border-radius: var(--radius);
      background: var(--surface);
      color: var(--ink);
      font-size: var(--text-sm);
    }
    h4 {
      margin-bottom: var(--space-3);
    }
    .list {
      list-style: none;
      margin: 0;
      padding: 0;
    }
    .list li {
      border-bottom: 1px solid var(--line-soft);
    }
    .list a {
      display: flex;
      justify-content: space-between;
      gap: var(--space-4);
      align-items: baseline;
      padding: var(--space-3) 0;
      text-decoration: none;
      color: var(--ink);
    }
    .list a:hover .name {
      color: var(--accent);
    }
    .name {
      font-weight: 600;
    }
    .meta {
      color: var(--muted);
      font-size: var(--text-sm);
    }
  `,
})
export class CompetitionPage {
  private readonly leagueReader = inject(LEAGUE_READER);
  private readonly teamReader = inject(TEAM_READER);
  private readonly seasonReader = inject(SEASON_READER);
  private readonly lookup = inject(LookupStore);
  protected readonly permissions = inject(Permissions);
  private readonly seo = inject(Seo);

  /** Empty means "now" — the current league_id list rather than a season. */
  protected readonly seasonId = signal('');
  protected readonly seasons = this.lookup.seasonsNewestFirst;

  readonly id = input.required<string>();
  private readonly page = signal<PageQuery>({ limit: 25, offset: 0 });

  protected readonly league = resource({
    params: () => ({ id: this.id() }),
    loader: ({ params }) => this.leagueReader.byId(params.id),
  });

  protected readonly clubs = resource({
    params: () => ({ id: this.id(), page: this.page() }),
    loader: ({ params }) => this.teamReader.list({ ...params.page, league_id: params.id }),
  });

  /**
   * Who contested this competition in the chosen season.
   *
   * Idle until a season is picked — `params` returning undefined skips the
   * loader entirely, so the default view costs no extra request.
   */
  protected readonly entrants = resource({
    params: () => {
      const season = this.seasonId();
      return season ? { season, league: this.id() } : undefined;
    },
    loader: async ({ params }) => {
      await this.lookup.loadTeams();
      return this.seasonReader.teams(params.season, params.league, { limit: 100 });
    },
  });

  protected readonly entrantsErrorMessage = computed(() =>
    messageFor(this.entrants.error(), 'Could not load the clubs for that season.'),
  );

  protected clubName(id: string): string {
    return this.lookup.teamName(id, 'Unknown club');
  }

  protected seasonLabel(): string {
    return this.lookup.seasonLabel(this.seasonId(), 'that season');
  }

  protected pickSeason(event: Event): void {
    this.seasonId.set((event.target as HTMLSelectElement).value);
  }

  constructor() {
    // The picker needs season labels before it can offer anything.
    void this.lookup.loadSeasons();

    effect(() => {
      const l = this.league.value();
      if (!l) return;

      this.seo.describe({
        title: l.name,
        description:
          `${l.name} — ${TYPE_LABELS[l.competition_type] ?? l.competition_type} in ${l.country}` +
          `${l.tier ? `, tier ${l.tier}` : ''}. The clubs competing in it.`,
        path: `/competitions/${l.id}`,
      });

      this.seo.structuredData({
        '@context': 'https://schema.org',
        '@type': 'SportsOrganization',
        name: l.name,
        sport: 'Football',
        location: l.country,
      });
    });
  }

  protected readonly errorMessage = computed(() => {
    const error = this.league.error();
    if (error instanceof ApiError && error.code === 'not_found') {
      return 'No competition with that id.';
    }
    return error instanceof Error ? error.message : 'Could not load the competition.';
  });

  protected typeLabel(type: string): string {
    return TYPE_LABELS[type] ?? type;
  }

  protected goTo(query: PageQuery): void {
    this.page.set(query);
  }
}
