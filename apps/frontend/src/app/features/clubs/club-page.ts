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
import { DatePipe } from '@angular/common';
import { RouterLink } from '@angular/router';

import { ApiError } from '../../core/api/api-error';
import { TEAM_READER, TEAM_WRITER } from '../../core/api/contracts';
import { LookupStore } from '../../core/api/lookup-store';
import { Permissions } from '../../core/auth/permissions';
import { Seo } from '../../core/seo/seo';
import { MoneyPipe } from '../../shared/pipes/money-pipe';
import { Actions } from '../../shared/ui/actions';
import { messageFor } from '../../shared/forms/submit';
import { Empty, ErrorState, Loading } from '../../shared/ui/states';

@Component({
  selector: 'app-club-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, DatePipe, MoneyPipe, Actions, Loading, Empty, ErrorState],
  template: `
    @if (club.isLoading()) {
      <main class="page"><app-loading message="Loading club…" /></main>
    } @else if (club.error()) {
      <main class="page"><app-error-state [message]="errorMessage()" /></main>
    } @else if (club.value(); as c) {
      <main class="page club">
        <header class="head">
          @if (c.league_id) {
            <p class="eyebrow">
              <a [routerLink]="['/competitions', c.league_id]">{{ leagueName(c.league_id) }}</a>
            </p>
          }
          <h1>{{ c.name }}</h1>

          <dl class="facts">
            @if (c.city) {
              <div>
                <dt>City</dt>
                <dd>{{ c.city }}</dd>
              </div>
            }
            @if (c.country) {
              <div>
                <dt>Country</dt>
                <dd>{{ c.country }}</dd>
              </div>
            }
            @if (c.stadium) {
              <div>
                <dt>Stadium</dt>
                <dd>{{ c.stadium }}</dd>
              </div>
            }
            @if (c.founded_year) {
              <div>
                <dt>Founded</dt>
                <dd class="tabular">{{ c.founded_year }}</dd>
              </div>
            }
          </dl>

          <!--
            Two permission levels, deliberately separated. An editor's grant
            covers the club's *contents* — its squad, its competition entries —
            while the club record itself is administrator-only, so "Edit club"
            appears for fewer people than "Add a player".
          -->
          @if (permissions.canEditTeam(c.id)) {
            <app-actions>
              @if (permissions.canAdminister()) {
                <a class="btn" [routerLink]="['/clubs', c.id, 'edit']">Edit club</a>
                <a class="btn" [routerLink]="['/admin/clubs', c.id, 'editors']">Editor access</a>
              }
              <a class="btn" routerLink="/players/new">Add a player</a>
              <a class="btn" [routerLink]="['/clubs', c.id, 'seasons', 'new']">
                Enter a competition
              </a>
            </app-actions>
          }
        </header>

        <section>
          <h4>Squad</h4>
          @if (squad.value()?.items?.length) {
            <div class="scroll-x">
              <table>
                <thead>
                  <tr>
                    <th scope="col">#</th>
                    <th scope="col">Player</th>
                    <th scope="col">Position</th>
                    <th scope="col">Nationality</th>
                    <th scope="col" class="right">Value</th>
                  </tr>
                </thead>
                <tbody>
                  @for (p of squad.value()!.items; track p.id) {
                    <tr>
                      <td class="tabular muted">{{ p.squad_number ?? '—' }}</td>
                      <td>
                        <a [routerLink]="['/players', p.id]">{{ p.name }}</a>
                      </td>
                      <td>{{ p.position }}</td>
                      <td class="muted">{{ p.nationality ?? '—' }}</td>
                      <td class="right tabular">
                        {{ p.market_value_minor | money: { currency: p.currency, compact: true } }}
                      </td>
                    </tr>
                  }
                </tbody>
              </table>
            </div>
          } @else {
            <p class="muted">No players recorded at this club.</p>
          }
        </section>

        <section>
          <h4>Managerial history</h4>
          @if (staff.value()?.items?.length) {
            <ul class="spells">
              @for (spell of staff.value()!.items; track spell.id) {
                <li>
                  <a [routerLink]="['/coaches', spell.coach_id]">Coach</a>
                  <span class="role">{{ spell.role.replace('_', ' ') }}</span>
                  <span class="dates tabular">
                    {{ spell.start_date | date: 'MMM y' }} –
                    <!-- An open spell is the current appointment, not missing data. -->
                    {{ spell.end_date ? (spell.end_date | date: 'MMM y') : 'present' }}
                  </span>
                </li>
              }
            </ul>
          } @else {
            <p class="muted">No managerial history recorded.</p>
          }
        </section>

        <section>
          <div class="section-head">
            <h4>Competitions</h4>
            @if (permissions.canEditTeam(c.id)) {
              <a class="add" [routerLink]="['/clubs', c.id, 'seasons', 'new']"
                >Enter a competition</a
              >
            }
          </div>

          <!--
            This is the club's *history*, distinct from the competition in the
            header — that one is teams.league_id, a single pointer to where
            they are now, which relegation overwrites. These entries survive it.
          -->
          @if (entries.isLoading()) {
            <app-loading [lines]="2" />
          } @else if (entries.value()?.items?.length) {
            <ul class="entries">
              @for (entry of entries.value()!.items; track entry.id) {
                <li>
                  <span class="season tabular">{{ seasonLabel(entry.season_id) }}</span>
                  <a class="comp" [routerLink]="['/competitions', entry.league_id]">
                    {{ leagueName(entry.league_id) }}
                  </a>
                  @if (permissions.canAdminister()) {
                    <!--
                      Admin-only, matching the API: withdrawing an entry after a
                      season is under way is rewriting history, not correcting a
                      club's own record.
                    -->
                    <button
                      type="button"
                      class="withdraw"
                      [disabled]="withdrawing() === entry.id"
                      (click)="withdraw(entry.id)"
                    >
                      {{ withdrawing() === entry.id ? 'Removing…' : 'Remove' }}
                    </button>
                  }
                </li>
              }
            </ul>
            @if (withdrawError()) {
              <app-error-state [message]="withdrawError()!" />
            }
          } @else {
            <app-empty
              message="No competition history recorded."
              hint="Entries record which competitions this club contested, season by season."
            />
          }
        </section>
      </main>
    }
  `,
  styles: `
    .club {
      padding-block: var(--space-6);
    }
    .head {
      border-bottom: 1px solid var(--line);
      padding-bottom: var(--space-5);
      margin-bottom: var(--space-6);
    }
    .eyebrow {
      font-size: var(--text-sm);
      margin-bottom: var(--space-2);
    }
    h1 {
      font-size: var(--text-3xl);
      margin-bottom: var(--space-5);
    }
    .facts {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr));
      gap: var(--space-4);
      margin: 0;
    }
    dt {
      font-size: var(--text-xs);
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--muted);
      margin-bottom: var(--space-1);
    }
    dd {
      margin: 0;
      font-weight: 600;
    }
    section {
      margin-bottom: var(--space-7);
    }
    h4 {
      margin-bottom: var(--space-3);
    }
    table {
      width: 100%;
      border-collapse: collapse;
      font-size: var(--text-sm);
    }
    th {
      text-align: left;
      font-size: var(--text-xs);
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--muted);
      font-weight: 700;
      padding: var(--space-3);
      background: var(--surface-2);
      white-space: nowrap;
    }
    td {
      padding: var(--space-3);
      border-bottom: 1px solid var(--line-soft);
    }
    .right {
      text-align: right;
    }
    .muted {
      color: var(--muted);
    }
    .spells {
      list-style: none;
      margin: 0;
      padding: 0;
    }
    .spells li {
      display: flex;
      gap: var(--space-4);
      align-items: baseline;
      padding: var(--space-3) 0;
      border-bottom: 1px solid var(--line-soft);
    }
    .role {
      font-family: var(--font-mono);
      font-size: 11px;
      color: var(--ink-soft);
    }
    .dates {
      color: var(--muted);
      font-size: var(--text-sm);
      margin-left: auto;
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
    .add {
      font-size: var(--text-sm);
    }
    .entries {
      list-style: none;
      margin: 0;
      padding: 0;
    }
    .entries li {
      display: flex;
      gap: var(--space-4);
      align-items: baseline;
      padding: var(--space-3) 0;
      border-bottom: 1px solid var(--line-soft);
    }
    .season {
      min-width: 5.5rem;
      color: var(--muted);
      font-size: var(--text-sm);
    }
    .comp {
      font-weight: 600;
    }
    .withdraw {
      margin-left: auto;
      background: none;
      border: 1px solid var(--line);
      border-radius: var(--radius);
      padding: var(--space-1) var(--space-3);
      font-size: var(--text-xs);
      color: var(--muted);
      cursor: pointer;
    }
    .withdraw:hover:not(:disabled) {
      border-color: var(--critical);
      color: var(--critical);
    }
    app-actions {
      display: block;
      margin-top: var(--space-5);
    }
  `,
})
export class ClubPage {
  private readonly reader = inject(TEAM_READER);
  private readonly writer = inject(TEAM_WRITER);
  private readonly lookup = inject(LookupStore);
  private readonly seo = inject(Seo);
  protected readonly permissions = inject(Permissions);

  readonly id = input.required<string>();

  protected readonly club = resource({
    params: () => ({ id: this.id() }),
    loader: async ({ params }) => {
      await this.lookup.loadLeagues();
      return this.reader.byId(params.id);
    },
  });

  protected readonly squad = resource({
    params: () => ({ id: this.id() }),
    loader: ({ params }) => this.reader.squad(params.id, { limit: 100 }),
  });

  protected readonly staff = resource({
    params: () => ({ id: this.id() }),
    loader: ({ params }) => this.reader.staff(params.id, { limit: 50 }),
  });

  /**
   * Which competitions this club has contested, newest season first.
   *
   * Season and competition names have to be resolved before the rows mean
   * anything — the API returns three ids and no names — so both lookups are
   * awaited here rather than leaving the list to render as dashes and fill in.
   */
  protected readonly entries = resource({
    params: () => ({ id: this.id() }),
    loader: async ({ params }) => {
      await Promise.all([this.lookup.loadSeasons(), this.lookup.loadLeagues()]);
      return this.reader.seasons(params.id, { limit: 100 });
    },
  });

  protected readonly withdrawing = signal<string | null>(null);
  protected readonly withdrawError = signal<string | null>(null);

  constructor() {
    effect(() => {
      const c = this.club.value();
      if (!c) return;

      const where = [c.city, c.country].filter(Boolean).join(', ');
      this.seo.describe({
        title: c.name,
        description: `${c.name}${where ? ` of ${where}` : ''} — squad, managerial history and transfers.`,
        path: `/clubs/${c.id}`,
      });

      this.seo.structuredData({
        '@context': 'https://schema.org',
        '@type': 'SportsTeam',
        name: c.name,
        alternateName: c.short_name,
        sport: 'Football',
        foundingDate: c.founded_year ? `${c.founded_year}` : undefined,
        location: where || undefined,
        logo: c.fan_badge_url,
        // The stadium is a place the team plays, which is not what
        // schema.org's `location` on an Organization means — so it is its own
        // SportsActivityLocation rather than folded into the address.
        homeLocation: c.stadium
          ? { '@type': 'SportsActivityLocation', name: c.stadium }
          : undefined,
      });
    });
  }

  protected seasonLabel(id: string): string {
    return this.lookup.seasonLabel(id, 'Unknown season');
  }

  protected async withdraw(entryId: string): Promise<void> {
    if (this.withdrawing()) return;
    if (!confirm('Remove this competition entry? It is part of the club’s recorded history.')) {
      return;
    }

    this.withdrawing.set(entryId);
    this.withdrawError.set(null);

    try {
      await this.writer.withdrawSeason(this.id(), entryId);
      this.entries.reload();
    } catch (error) {
      this.withdrawError.set(messageFor(error, 'Could not remove the entry.'));
    } finally {
      this.withdrawing.set(null);
    }
  }

  protected readonly errorMessage = computed(() => {
    const error = this.club.error();
    if (error instanceof ApiError && error.code === 'not_found') return 'No club with that id.';
    return error instanceof Error ? error.message : 'Could not load the club.';
  });

  protected leagueName(id: string | null): string {
    return this.lookup.leagueName(id, 'Competition');
  }
}
