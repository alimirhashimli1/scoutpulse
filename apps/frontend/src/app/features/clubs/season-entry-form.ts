import { ChangeDetectionStrategy, Component, computed, inject, input, resource, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';

import {
  LEAGUE_READER,
  SEASON_READER,
  TEAM_READER,
  TEAM_WRITER,
} from '../../core/api/contracts';
import { Permissions } from '../../core/auth/permissions';
import { TeamSeason } from '../../core/models/football';
import { Field } from '../../shared/forms/field';
import { messageFor, requestIdFor } from '../../shared/forms/submit';
import { ErrorState, Loading } from '../../shared/ui/states';

/**
 * Enter a club in a competition for a season.
 *
 * This is the club's *history*, as distinct from `teams.league_id`, which is a
 * single pointer to where they are now. A club plays a league and two cups in
 * one season; only the entries record that, and only they survive relegation.
 *
 * Unusually among the club-level writes, this one is not administrator-only:
 * `Enter` checks `RequireTeam` against the club, so a club's own editor may
 * file it. Withdrawing an entry *is* administrator-only — by the time a season
 * is under way, removing a club from a competition it played in is rewriting
 * history rather than correcting a record.
 */
@Component({
  selector: 'app-season-entry-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, Field, Loading, ErrorState],
  template: `
    <main class="page form-page">
      @if (club.isLoading()) {
        <app-loading message="Loading club…" />
      } @else if (club.error()) {
        <app-error-state [message]="loadErrorMessage()" />
      } @else if (club.value(); as c) {
        <header class="head">
          <p class="eyebrow">Enter a competition</p>
          <h1>{{ c.name }}</h1>
        </header>

        @if (!seasons.value()?.items?.length) {
          <app-error-state
            message="There are no seasons yet. A season has to exist before a club can be entered in one."
          />
        } @else {
          <form (ngSubmit)="save()" novalidate>
            <section class="grid">
              <app-field for="season" label="Season" [error]="fieldError('season_id')">
                <select id="season" name="season" [(ngModel)]="seasonId">
                  <option value="">— choose —</option>
                  @for (season of seasons.value()!.items; track season.id) {
                    <option [value]="season.id">{{ season.label }}</option>
                  }
                </select>
              </app-field>

              <app-field for="league" label="Competition" [error]="fieldError('league_id')">
                <select id="league" name="league" [(ngModel)]="leagueId">
                  <option value="">— choose —</option>
                  @for (league of leagues.value()?.items ?? []; track league.id) {
                    <option [value]="league.id">{{ league.name }}</option>
                  }
                </select>
              </app-field>
            </section>

            @if (!permitted()) {
              <p class="notice">Only an editor of this club, or an administrator, may enter it.</p>
            }

            @if (saveError()) {
              <app-error-state [message]="saveError()!" [requestId]="saveRequestId()" />
            }

            <footer class="actions">
              <button class="btn primary" type="submit" [disabled]="busy() || !permitted()">
                {{ busy() ? 'Entering…' : 'Enter competition' }}
              </button>
              <a class="btn" [routerLink]="['/clubs', id()]">Cancel</a>
            </footer>
          </form>
        }
      }
    </main>
  `,
  styles: `
    .form-page { max-width: 36rem; padding-block: var(--space-6) var(--space-8); }
    .head { margin-bottom: var(--space-6); }
    .eyebrow {
      font-family: var(--font-mono); font-size: var(--text-xs);
      letter-spacing: 0.12em; text-transform: uppercase;
      color: var(--muted); margin-bottom: var(--space-2);
    }
    h1 { font-size: var(--text-2xl); }
    form { display: flex; flex-direction: column; gap: var(--space-5); }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(13rem, 1fr));
      gap: var(--space-4);
    }
    .notice {
      padding: var(--space-3) var(--space-4);
      background: var(--warning-soft);
      border-radius: var(--radius);
      font-size: var(--text-sm);
      color: var(--ink-soft);
    }
    .actions { display: flex; gap: var(--space-3); }
  `,
})
export class SeasonEntryForm {
  private readonly teams = inject(TEAM_READER);
  private readonly writer = inject(TEAM_WRITER);
  private readonly seasonReader = inject(SEASON_READER);
  private readonly leagueReader = inject(LEAGUE_READER);
  private readonly permissions = inject(Permissions);
  private readonly router = inject(Router);

  readonly id = input.required<string>();

  protected readonly seasonId = signal('');
  protected readonly leagueId = signal('');

  protected readonly busy = signal(false);
  protected readonly saveError = signal<string | null>(null);
  protected readonly saveRequestId = signal<string | null>(null);
  private readonly errors = signal<Record<string, string>>({});

  protected readonly club = resource({
    params: () => ({ id: this.id() }),
    loader: ({ params }) => this.teams.byId(params.id),
  });

  protected readonly seasons = resource({
    loader: () => this.seasonReader.list({ limit: 100 }),
  });

  protected readonly leagues = resource({
    loader: () => this.leagueReader.list({ limit: 100 }),
  });

  protected readonly permitted = computed(() => this.permissions.canEditTeam(this.id()));

  protected readonly loadErrorMessage = computed(() =>
    messageFor(this.club.error(), 'Could not load the club.'),
  );

  protected fieldError(field: string): string | null {
    return this.errors()[field] ?? null;
  }

  protected async save(): Promise<void> {
    if (this.busy()) return;

    const errors: Record<string, string> = {};
    if (!this.seasonId()) errors['season_id'] = 'Choose a season.';
    if (!this.leagueId()) errors['league_id'] = 'Choose a competition.';

    this.errors.set(errors);
    if (Object.keys(errors).length > 0) return;

    const entry: Partial<TeamSeason> = {
      team_id: this.id(),
      season_id: this.seasonId(),
      league_id: this.leagueId(),
    };

    this.busy.set(true);
    this.saveError.set(null);
    this.saveRequestId.set(null);

    try {
      await this.writer.enterSeason(this.id(), entry);
      await this.router.navigate(['/clubs', this.id()]);
    } catch (error) {
      this.saveError.set(messageFor(error, 'Could not enter the competition.'));
      this.saveRequestId.set(requestIdFor(error));
    } finally {
      this.busy.set(false);
    }
  }
}
