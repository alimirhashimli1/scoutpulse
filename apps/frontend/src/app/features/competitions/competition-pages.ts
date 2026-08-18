import { ChangeDetectionStrategy, Component, computed, inject, input, resource, signal } from '@angular/core';
import { RouterLink } from '@angular/router';

import { ApiError } from '../../core/api/api-error';
import { LEAGUE_READER, TEAM_READER } from '../../core/api/contracts';
import { PageQuery } from '../../core/api/page';
import { Permissions } from '../../core/auth/permissions';
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
                  @if (league.tier) { <span class="muted">· tier {{ league.tier }}</span> }
                </span>
              </a>
            </li>
          }
        </ul>
        <app-paginator [page]="leagues.value()!" label="competition results" (pageChange)="goTo($event)" />
      }
    </main>
  `,
  styles: `
    .masthead {
      display: flex; justify-content: space-between; align-items: flex-end;
      gap: var(--space-4); flex-wrap: wrap;
      padding-block: var(--space-6) var(--space-5);
    }
    h1 { margin-bottom: var(--space-2); }
    .standfirst { color: var(--ink-soft); }
    .list { list-style: none; margin: 0; padding: 0; }
    .list li { border-bottom: 1px solid var(--line-soft); }
    .list a {
      display: flex; justify-content: space-between; gap: var(--space-4);
      align-items: baseline; padding: var(--space-3) 0;
      text-decoration: none; color: var(--ink);
    }
    .list a:hover .name { color: var(--accent); }
    .name { font-weight: 600; }
    .meta { color: var(--ink-soft); font-size: var(--text-sm); }
    .muted { color: var(--muted); }
  `,
})
export class CompetitionList {
  private readonly reader = inject(LEAGUE_READER);
  protected readonly permissions = inject(Permissions);
  private readonly page = signal<PageQuery>({ limit: 25, offset: 0 });

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
 * A competition and the clubs currently in it.
 *
 * "Currently" is the honest word: `teams.league_id` is the club's present
 * competition, not a historical record. Which clubs contested it in a given
 * season lives in team_seasons, and needs a season to ask about — a page for
 * that belongs with a season picker, not here.
 *
 * There is no table. There is no match data, so there is nothing to compute a
 * standing from, and a placeholder implying otherwise would be a lie.
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
            {{ l.country }}@if (l.tier) { <span> · tier {{ l.tier }}</span> }
          </p>
          @if (permissions.canAdminister()) {
            <p class="edit"><a class="btn" [routerLink]="['/competitions', l.id, 'edit']">Edit</a></p>
          }
        </header>

        <section>
          <h4>Clubs</h4>
          @if (clubs.isLoading()) {
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
    .head { border-bottom: 1px solid var(--line); padding-block: var(--space-6) var(--space-5); margin-bottom: var(--space-6); }
    .eyebrow {
      font-family: var(--font-mono); font-size: var(--text-xs);
      letter-spacing: 0.12em; text-transform: uppercase;
      color: var(--muted); margin-bottom: var(--space-2);
    }
    h1 { font-size: var(--text-3xl); margin-bottom: var(--space-2); }
    .standfirst { color: var(--ink-soft); }
    .edit { margin-top: var(--space-4); }
    h4 { margin-bottom: var(--space-3); }
    .list { list-style: none; margin: 0; padding: 0; }
    .list li { border-bottom: 1px solid var(--line-soft); }
    .list a {
      display: flex; justify-content: space-between; gap: var(--space-4);
      align-items: baseline; padding: var(--space-3) 0;
      text-decoration: none; color: var(--ink);
    }
    .list a:hover .name { color: var(--accent); }
    .name { font-weight: 600; }
    .meta { color: var(--muted); font-size: var(--text-sm); }
  `,
})
export class CompetitionPage {
  private readonly leagueReader = inject(LEAGUE_READER);
  private readonly teamReader = inject(TEAM_READER);
  protected readonly permissions = inject(Permissions);

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
