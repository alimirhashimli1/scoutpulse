import { ChangeDetectionStrategy, Component, computed, inject, input, resource } from '@angular/core';
import { DatePipe } from '@angular/common';
import { RouterLink } from '@angular/router';

import { ApiError } from '../../core/api/api-error';
import { PLAYER_READER } from '../../core/api/contracts';
import { LookupStore } from '../../core/api/lookup-store';
import { Permissions } from '../../core/auth/permissions';
import { MoneyPipe } from '../../shared/pipes/money-pipe';
import { Actions } from '../../shared/ui/actions';
import { ErrorState, Loading } from '../../shared/ui/states';
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
  imports: [RouterLink, DatePipe, MoneyPipe, ValueChart, Actions, Loading, ErrorState],
  template: `
    @if (player.isLoading()) {
      <main class="page"><app-loading message="Loading player…" /></main>
    } @else if (player.error()) {
      <main class="page"><app-error-state [message]="errorMessage()" [requestId]="errorRequestId()" /></main>
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
              <dd class="tabular">{{ p.market_value_minor | money: { currency: p.currency, compact: true } }}</dd>
            </div>
            @if (p.nationality) {
              <div><dt>Nationality</dt><dd>{{ p.nationality }}</dd></div>
            }
            @if (p.date_of_birth) {
              <div><dt>Born</dt><dd>{{ p.date_of_birth | date: 'd MMM y' }}</dd></div>
            }
            @if (p.squad_number) {
              <div><dt>Squad number</dt><dd class="tabular">{{ p.squad_number }}</dd></div>
            }
            @if (p.preferred_foot) {
              <div><dt>Foot</dt><dd>{{ p.preferred_foot }}</dd></div>
            }
            @if (p.height_cm) {
              <div><dt>Height</dt><dd class="tabular">{{ p.height_cm }} cm</dd></div>
            }
            @if (p.contract_until) {
              <div><dt>Contract until</dt><dd>{{ p.contract_until | date: 'MMM y' }}</dd></div>
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
                    <th>Date</th><th>From</th><th>To</th><th>Type</th><th class="right">Fee</th>
                    <th><span class="visually-hidden">Actions</span></th>
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
      </main>
    }
  `,
  styles: `
    .profile { padding-block: var(--space-6); }
    .head { border-bottom: 1px solid var(--line); padding-bottom: var(--space-5); margin-bottom: var(--space-6); }
    .eyebrow {
      font-family: var(--font-mono);
      font-size: var(--text-xs);
      letter-spacing: 0.12em;
      text-transform: uppercase;
      color: var(--muted);
      margin-bottom: var(--space-2);
    }
    h1 { font-size: var(--text-3xl); margin-bottom: var(--space-5); }
    .facts {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr));
      gap: var(--space-4);
      margin: 0;
    }
    .facts div { margin: 0; }
    dt {
      font-size: var(--text-xs);
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--muted);
      margin-bottom: var(--space-1);
    }
    dd { margin: 0; font-weight: 600; }
    section { margin-bottom: var(--space-7); }
    h4 { margin-bottom: var(--space-3); }
    table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
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
    td { padding: var(--space-3); border-bottom: 1px solid var(--line-soft); }
    .right { text-align: right; }
    .muted { color: var(--muted); }
    .type { font-family: var(--font-mono); font-size: 11px; color: var(--ink-soft); }
    .correct { font-size: var(--text-xs); }
    app-actions { display: block; margin-top: var(--space-5); }
  `,
})
export class PlayerPage {
  private readonly reader = inject(PLAYER_READER);
  private readonly lookup = inject(LookupStore);
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

  protected clubName(id: string | null): string {
    return this.lookup.teamName(id, '—');
  }
}
