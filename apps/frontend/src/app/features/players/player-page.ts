import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  resource,
} from '@angular/core';
import { DatePipe } from '@angular/common';
import { RouterLink } from '@angular/router';

import { ApiError } from '../../core/api/api-error';
import { PLAYER_READER } from '../../core/api/contracts';
import { LookupStore } from '../../core/api/lookup-store';
import { Permissions } from '../../core/auth/permissions';
import { Player } from '../../core/models/football';
import { Seo } from '../../core/seo/seo';
import { MoneyPipe } from '../../shared/pipes/money-pipe';
import { Actions } from '../../shared/ui/actions';
import { ErrorState, Loading } from '../../shared/ui/states';
import { PlayerNotes } from './player-notes';
import { ValueChart } from './value-chart';

/**
 * A player profile: who they are, where they are, and how they got there.
 *
 * The career and the valuation curve are the point. A flat CRUD view would
 * show the current club and stop, which is exactly what the temporal model
 * exists to avoid.
 */
@Component({
  selector: 'app-player-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, DatePipe, MoneyPipe, ValueChart, Actions, PlayerNotes, Loading, ErrorState],
  template: `
    @if (player.isLoading()) {
      <main class="page"><app-loading message="Loading player…" /></main>
    } @else if (player.error()) {
      <main class="page">
        <app-error-state [message]="errorMessage()" [requestId]="errorRequestId()" />
      </main>
    } @else if (player.value(); as p) {
      <main class="page profile">
        <header class="head">
          <p class="eyebrow">{{ p.position }}</p>
          <h1>{{ p.name }}</h1>

          <dl class="facts">
            <div>
              <dt>Club</dt>
              <dd>
                @if (p.team_id) {
                  <a [routerLink]="['/clubs', p.team_id]">{{ clubName(p.team_id) }}</a>
                } @else {
                  <!-- Not missing data: a free agent has no club, and saying so
                       is more useful than an empty cell. -->
                  <span class="muted">Free agent</span>
                }
              </dd>
            </div>
            <div>
              <dt>Market value</dt>
              <dd class="tabular">
                {{ p.market_value_minor | money: { currency: p.currency, compact: true } }}
              </dd>
            </div>
            @if (p.nationalities.length) {
              <div>
                <!--
                  Plural only when it is: a dual national is a fact worth
                  showing, and "Nationalities: Norway" reads as a mistake.
                -->
                <dt>{{ p.nationalities.length > 1 ? 'Nationalities' : 'Nationality' }}</dt>
                <dd>{{ p.nationalities.join(' / ') }}</dd>
              </div>
            }
            @if (p.secondary_positions.length) {
              <div>
                <dt>Also plays</dt>
                <dd>{{ p.secondary_positions.join(', ') }}</dd>
              </div>
            }
            @if (p.date_of_birth) {
              <div>
                <dt>Born</dt>
                <dd>{{ p.date_of_birth | date: 'd MMM y' }}</dd>
              </div>
            }
            @if (p.squad_number) {
              <div>
                <dt>Squad number</dt>
                <dd class="tabular">{{ p.squad_number }}</dd>
              </div>
            }
            @if (p.preferred_foot) {
              <div>
                <dt>Foot</dt>
                <dd>{{ p.preferred_foot }}</dd>
              </div>
            }
            @if (p.height_cm) {
              <div>
                <dt>Height</dt>
                <dd class="tabular">{{ p.height_cm }} cm</dd>
              </div>
            }
            @if (p.contract_until) {
              <div>
                <dt>Contract until</dt>
                <dd>{{ p.contract_until | date: 'MMM y' }}</dd>
              </div>
            }
          </dl>

          <!--
            Shown only where the API would accept it. "Record a transfer" is
            the only control that changes a player's club — there is no club
            field on the edit form, by design.
          -->
          @if (permissions.canEditPlayer(p)) {
            <app-actions>
              <a class="btn" [routerLink]="['/players', p.id, 'edit']">Edit details</a>
              <a class="btn primary" [routerLink]="['/players', p.id, 'transfer']">
                Record a transfer
              </a>
              @if (permissions.canAdminister()) {
                <a class="btn" [routerLink]="['/players', p.id, 'values', 'new']">Record a value</a>
              }
            </app-actions>
          }
        </header>

        @if (percentages(p).length) {
          <section>
            <h4>Match percentages</h4>
            <!--
              Recorded by a scout, not computed: there is no match data behind
              these. A metric that was never entered is absent rather than
              shown as zero — "no data" and "won none of his duels" are very
              different claims about a player.
            -->
            <ul class="metrics">
              @for (metric of percentages(p); track metric.label) {
                <li>
                  <span class="metric-label">{{ metric.label }}</span>
                  <span class="bar" aria-hidden="true">
                    <span class="fill" [style.width.%]="metric.value"></span>
                  </span>
                  <span class="metric-value tabular">{{ metric.value }}%</span>
                </li>
              }
            </ul>
          </section>
        }

        @if (p.strengths.length || p.weaknesses.length) {
          <section>
            <h4>Assessment</h4>
            <div class="assessment">
              @if (p.strengths.length) {
                <div class="column strengths">
                  <h5>Strengths</h5>
                  <ul>
                    @for (item of p.strengths; track item) {
                      <li>{{ item }}</li>
                    }
                  </ul>
                </div>
              }
              @if (p.weaknesses.length) {
                <div class="column weaknesses">
                  <h5>Weaknesses</h5>
                  <ul>
                    @for (item of p.weaknesses; track item) {
                      <li>{{ item }}</li>
                    }
                  </ul>
                </div>
              }
            </div>
          </section>
        }

        <section>
          <h4>Market value</h4>
          @if (values.value(); as page) {
            <app-value-chart [values]="page.items" />
          }
        </section>

        <section>
          <h4>Career</h4>
          @if (transfers.value()?.items?.length) {
            <div class="scroll-x">
              <table>
                <thead>
                  <tr>
                    <th scope="col">Date</th>
                    <th scope="col">From</th>
                    <th scope="col">To</th>
                    <th scope="col">Type</th>
                    <th scope="col" class="right">Fee</th>
                    <th scope="col"><span class="visually-hidden">Actions</span></th>
                  </tr>
                </thead>
                <tbody>
                  @for (t of transfers.value()!.items; track t.id) {
                    <tr>
                      <td class="tabular muted">{{ t.transfer_date | date: 'd MMM y' }}</td>
                      <td>{{ clubName(t.from_team_id) }}</td>
                      <td>{{ clubName(t.to_team_id) }}</td>
                      <td class="type">{{ t.transfer_type }}</td>
                      <td class="right tabular">
                        {{ t.fee_minor | money: { currency: t.currency, compact: true } }}
                      </td>
                      <td class="right">
                        <!--
                          Either club's editor may correct a move, which is why
                          this is checked per row rather than once for the page:
                          a player's history can span clubs the reader holds
                          different rights over.
                        -->
                        @if (permissions.canMoveBetween(t.from_team_id, t.to_team_id)) {
                          <a class="correct" [routerLink]="['/transfers', t.id, 'edit']">Correct</a>
                        }
                      </td>
                    </tr>
                  }
                </tbody>
              </table>
            </div>
          } @else {
            <p class="muted">No moves recorded.</p>
          }
        </section>

        <app-player-notes [playerId]="p.id" />
      </main>
    }
  `,
  styles: `
    .profile {
      padding-block: var(--space-6);
    }
    .head {
      border-bottom: 1px solid var(--line);
      padding-bottom: var(--space-5);
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
      margin-bottom: var(--space-5);
    }
    .facts {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr));
      gap: var(--space-4);
      margin: 0;
    }
    .facts div {
      margin: 0;
    }
    dt {
      font-size: var(--text-xs);
      letter-spacing: 0.08em;
      text-transform: uppercase;
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
    .type {
      font-family: var(--font-mono);
      font-size: 11px;
      color: var(--ink-soft);
    }
    .metrics {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      gap: var(--space-3);
      max-width: 34rem;
    }
    .metrics li {
      display: grid;
      grid-template-columns: 10rem 1fr 3.5rem;
      gap: var(--space-3);
      align-items: center;
    }
    .metric-label {
      font-size: var(--text-sm);
      color: var(--ink-soft);
    }
    .bar {
      height: 6px;
      border-radius: 999px;
      background: var(--surface-2);
      overflow: hidden;
    }
    .fill {
      display: block;
      height: 100%;
      background: var(--accent);
    }
    .metric-value {
      text-align: right;
      font-size: var(--text-sm);
    }
    .assessment {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
      gap: var(--space-5);
    }
    .column h5 {
      font-size: var(--text-xs);
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--muted);
      margin-bottom: var(--space-2);
    }
    .column ul {
      margin: 0;
      padding-left: var(--space-5);
    }
    .column li {
      margin-bottom: var(--space-2);
    }
    .strengths h5 {
      color: var(--positive);
    }
    .weaknesses h5 {
      color: var(--warning);
    }
    @media (max-width: 34rem) {
      .metrics li {
        grid-template-columns: 1fr 3.5rem;
      }
      .bar {
        grid-column: 1 / -1;
      }
    }
    .correct {
      font-size: var(--text-xs);
    }
    app-actions {
      display: block;
      margin-top: var(--space-5);
    }
  `,
})
export class PlayerPage {
  private readonly reader = inject(PLAYER_READER);
  private readonly lookup = inject(LookupStore);
  private readonly seo = inject(Seo);
  protected readonly permissions = inject(Permissions);

  /** Bound from the route parameter. */
  readonly id = input.required<string>();

  protected readonly player = resource({
    params: () => ({ id: this.id() }),
    loader: async ({ params }) => {
      await this.lookup.loadTeams();
      return this.reader.byId(params.id);
    },
  });

  protected readonly transfers = resource({
    params: () => ({ id: this.id() }),
    loader: ({ params }) => this.reader.transfers(params.id, { limit: 50 }),
  });

  protected readonly values = resource({
    params: () => ({ id: this.id() }),
    loader: ({ params }) => this.reader.marketValues(params.id, { limit: 100 }),
  });

  constructor() {
    // Runs during the server render too, so the title, description, canonical
    // and JSON-LD are in the first response rather than appearing after
    // hydration — which is the entire reason this page is server-rendered.
    effect(() => {
      const p = this.player.value();
      if (!p) return;

      const club = p.team_id ? this.lookup.teamName(p.team_id, 'a club') : 'a free agent';
      this.seo.describe({
        title: p.name,
        description:
          `${p.name} — ${p.position}${p.nationalities.length ? `, ${p.nationalities[0]}` : ''}. ` +
          `Career, transfers and market value history at ${club}.`,
        path: `/players/${p.id}`,
        type: 'profile',
      });

      // schema.org/Person. `athlete` is not a schema.org type, so the club is
      // expressed as memberOf rather than invented as a property.
      this.seo.structuredData({
        '@context': 'https://schema.org',
        '@type': 'Person',
        name: p.name,
        givenName: p.first_name,
        familyName: p.last_name,
        birthDate: p.date_of_birth?.slice(0, 10),
        // schema.org takes one value here; the primary nationality is it.
        nationality: p.nationalities[0],
        jobTitle: p.position,
        memberOf: p.team_id
          ? { '@type': 'SportsTeam', name: this.lookup.teamName(p.team_id, 'Club') }
          : undefined,
      });
    });
  }

  protected readonly errorMessage = computed(() => {
    const error = this.player.error();
    if (error instanceof ApiError && error.code === 'not_found') {
      return 'No player with that id.';
    }
    return error instanceof Error ? error.message : 'Could not load the player.';
  });

  protected readonly errorRequestId = computed(() => {
    const error = this.player.error();
    return error instanceof ApiError ? (error.requestId ?? null) : null;
  });

  /**
   * The recorded percentages, in a fixed order, skipping any not entered.
   *
   * A list rather than four bound fields, so the template does not repeat the
   * same block four times — and an unrecorded metric is simply absent: no
   * empty row, and never a zero standing in for missing data.
   */
  protected percentages(p: Player): { label: string; value: number }[] {
    const candidates: { label: string; value: number | undefined }[] = [
      { label: 'Duels won', value: p.duels_won_pct },
      { label: 'Passes completed', value: p.pass_completion_pct },
      { label: 'Shots on target', value: p.shots_on_target_pct },
      { label: 'Headers won', value: p.aerial_duels_won_pct },
    ];

    return (
      candidates
        .filter((m): m is { label: string; value: number } => typeof m.value === 'number')
        // One decimal at most: 87.4% is a measurement, 87.42857% is arithmetic
        // leaking into the page.
        .map((m) => ({ label: m.label, value: Math.round(m.value * 10) / 10 }))
    );
  }

  protected clubName(id: string | null): string {
    return this.lookup.teamName(id, '—');
  }
}
