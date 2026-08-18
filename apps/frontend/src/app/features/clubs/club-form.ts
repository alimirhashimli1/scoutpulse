import { ChangeDetectionStrategy, Component, computed, effect, inject, input, resource, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';

import { LEAGUE_READER, TEAM_READER, TEAM_WRITER } from '../../core/api/contracts';
import { Team } from '../../core/models/football';
import { Field } from '../../shared/forms/field';
import { messageFor, requestIdFor } from '../../shared/forms/submit';
import { ErrorState, Loading } from '../../shared/ui/states';

/**
 * Create or edit a club. Administrators only — the whole form, not just the
 * competition field.
 *
 * `TeamService.CreateTeam` and `UpdateTeam` both call `RequireAdmin`, so an
 * editor holding a grant for a club still cannot rename it. The grant covers
 * the club's *contents* — its players, its transfers, the competitions it
 * enters — not the club record itself. docs/FRONTEND.md described this as
 * "league_id admin-only", which understated it.
 *
 * `league_id` is the club's *current* primary competition, a single mutable
 * pointer. Which competitions it contested in past seasons lives in
 * team_seasons and is edited from the club page, not here.
 */
@Component({
  selector: 'app-club-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, Field, Loading, ErrorState],
  template: `
    <main class="page form-page">
      @if (loading()) {
        <app-loading message="Loading club…" />
      } @else if (existing.error()) {
        <app-error-state [message]="loadErrorMessage()" />
      } @else {
        <header class="head">
          <p class="eyebrow">{{ isEdit() ? 'Edit club' : 'New club' }}</p>
          <h1>{{ isEdit() ? name() || 'Club' : 'Add a club' }}</h1>
        </header>

        <form (ngSubmit)="save()" novalidate>
          <section class="grid">
            <app-field for="name" label="Name" [error]="fieldError('name')">
              <input id="name" name="name" required [(ngModel)]="name" />
            </app-field>

            <app-field for="short" label="Short name" [optional]="true" hint="For tables and feeds.">
              <input id="short" name="short" [(ngModel)]="shortName" />
            </app-field>

            <app-field
              for="league"
              label="Competition"
              [optional]="true"
              hint="The club's current primary competition."
            >
              <select id="league" name="league" [(ngModel)]="leagueId">
                <option value="">— none —</option>
                @for (league of leagues.value()?.items ?? []; track league.id) {
                  <option [value]="league.id">{{ league.name }}</option>
                }
              </select>
            </app-field>

            <app-field
              for="founded"
              label="Founded"
              [optional]="true"
              [error]="fieldError('founded_year')"
            >
              <input
                id="founded"
                name="founded"
                type="number"
                min="1850"
                [max]="currentYear"
                [(ngModel)]="foundedYear"
              />
            </app-field>

            <app-field for="stadium" label="Stadium" [optional]="true">
              <input id="stadium" name="stadium" [(ngModel)]="stadium" />
            </app-field>

            <app-field for="city" label="City" [optional]="true">
              <input id="city" name="city" [(ngModel)]="city" />
            </app-field>

            <app-field for="country" label="Country" [optional]="true">
              <input id="country" name="country" [(ngModel)]="country" />
            </app-field>

            <app-field for="badge" label="Badge URL" [optional]="true">
              <input id="badge" name="badge" type="url" [(ngModel)]="badgeUrl" />
            </app-field>
          </section>

          @if (saveError()) {
            <app-error-state [message]="saveError()!" [requestId]="saveRequestId()" />
          }

          <footer class="actions">
            <button class="btn primary" type="submit" [disabled]="busy()">
              {{ busy() ? 'Saving…' : isEdit() ? 'Save changes' : 'Create club' }}
            </button>
            <a class="btn" [routerLink]="cancelTarget()">Cancel</a>
          </footer>
        </form>
      }
    </main>
  `,
  styles: `
    .form-page { max-width: 44rem; padding-block: var(--space-6) var(--space-8); }
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
      grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
      gap: var(--space-4);
    }
    .actions { display: flex; gap: var(--space-3); }
    @media (max-width: 30rem) {
      .actions { flex-direction: column; }
      .actions .btn { justify-content: center; }
    }
  `,
})
export class ClubForm {
  private readonly reader = inject(TEAM_READER);
  private readonly writer = inject(TEAM_WRITER);
  private readonly leagueReader = inject(LEAGUE_READER);
  private readonly router = inject(Router);

  readonly id = input<string | undefined>(undefined);

  protected readonly currentYear = new Date().getFullYear();

  protected readonly name = signal('');
  protected readonly shortName = signal('');
  protected readonly leagueId = signal('');
  protected readonly foundedYear = signal<number | null>(null);
  protected readonly stadium = signal('');
  protected readonly city = signal('');
  protected readonly country = signal('');
  protected readonly badgeUrl = signal('');

  protected readonly busy = signal(false);
  protected readonly saveError = signal<string | null>(null);
  protected readonly saveRequestId = signal<string | null>(null);
  private readonly errors = signal<Record<string, string>>({});

  protected readonly isEdit = computed(() => !!this.id());

  protected readonly existing = resource({
    params: () => {
      const id = this.id();
      return id ? { id } : undefined;
    },
    loader: ({ params }) => this.reader.byId(params.id),
  });

  protected readonly leagues = resource({
    loader: () => this.leagueReader.list({ limit: 100 }),
  });

  protected readonly loading = computed(() => this.isEdit() && this.existing.isLoading());
  protected readonly loadErrorMessage = computed(() =>
    messageFor(this.existing.error(), 'Could not load the club.'),
  );

  constructor() {
    effect(() => {
      const club = this.existing.value();
      if (club) this.fill(club);
    });
  }

  protected cancelTarget(): string {
    return this.id() ? `/clubs/${this.id()}` : '/clubs';
  }

  protected fieldError(field: string): string | null {
    return this.errors()[field] ?? null;
  }

  protected async save(): Promise<void> {
    if (this.busy()) return;

    const body = this.build();
    if (!body) return;

    this.busy.set(true);
    this.saveError.set(null);
    this.saveRequestId.set(null);

    try {
      const id = this.id();
      const saved = id ? await this.writer.update(id, body) : await this.writer.create(body);
      await this.router.navigate(['/clubs', saved.id]);
    } catch (error) {
      this.saveError.set(messageFor(error));
      this.saveRequestId.set(requestIdFor(error));
    } finally {
      this.busy.set(false);
    }
  }

  private build(): Partial<Team> | null {
    const errors: Record<string, string> = {};

    const name = this.name().trim();
    if (!name) errors['name'] = 'A name is required.';

    const founded = this.foundedYear();
    if (founded !== null && (founded < 1850 || founded > this.currentYear)) {
      errors['founded_year'] = `Between 1850 and ${this.currentYear}.`;
    }

    this.errors.set(errors);
    if (Object.keys(errors).length > 0) return null;

    return {
      name,
      short_name: blankToUndefined(this.shortName()),
      // Null rather than undefined: clearing a club's competition has to be
      // expressible, and an omitted field would leave the old value in place.
      league_id: this.leagueId() || null,
      founded_year: founded ?? undefined,
      stadium: blankToUndefined(this.stadium()),
      city: blankToUndefined(this.city()),
      country: blankToUndefined(this.country()),
      fan_badge_url: blankToUndefined(this.badgeUrl()),
    };
  }

  private fill(club: Team): void {
    this.name.set(club.name);
    this.shortName.set(club.short_name ?? '');
    this.leagueId.set(club.league_id ?? '');
    this.foundedYear.set(club.founded_year ?? null);
    this.stadium.set(club.stadium ?? '');
    this.city.set(club.city ?? '');
    this.country.set(club.country ?? '');
    this.badgeUrl.set(club.fan_badge_url ?? '');
  }
}

function blankToUndefined(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed === '' ? undefined : trimmed;
}
