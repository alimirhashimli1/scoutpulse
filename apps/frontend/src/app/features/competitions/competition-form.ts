import { ChangeDetectionStrategy, Component, computed, effect, inject, input, resource, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';

import { LEAGUE_READER, LEAGUE_WRITER } from '../../core/api/contracts';
import { CompetitionType, League } from '../../core/models/football';
import { Field } from '../../shared/forms/field';
import { messageFor, requestIdFor } from '../../shared/forms/submit';
import { ErrorState, Loading } from '../../shared/ui/states';

const TYPES: { value: CompetitionType; label: string }[] = [
  { value: 'league', label: 'League' },
  { value: 'domestic_cup', label: 'Domestic cup' },
  { value: 'international_cup', label: 'International cup' },
  { value: 'super_cup', label: 'Super cup' },
];

/**
 * Create or edit a competition. Administrators only.
 *
 * Not in the phase-4 list, which went from clubs straight to coaches — but the
 * club form asks which competition a club is in, and with no way to create one
 * that dropdown can only ever be empty on a fresh database. Added for that
 * reason rather than for completeness.
 *
 * `tier` is meaningful for leagues and not for cups: the Süper Lig is tier 1,
 * the Turkish Cup is not tiered at all. The API allows a tier on any type, so
 * this hides rather than forbids it.
 */
@Component({
  selector: 'app-competition-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, Field, Loading, ErrorState],
  template: `
    <main class="page form-page">
      @if (loading()) {
        <app-loading message="Loading competition…" />
      } @else if (existing.error()) {
        <app-error-state [message]="loadErrorMessage()" />
      } @else {
        <header class="head">
          <p class="eyebrow">{{ isEdit() ? 'Edit competition' : 'New competition' }}</p>
          <h1>{{ isEdit() ? name() || 'Competition' : 'Add a competition' }}</h1>
        </header>

        <form (ngSubmit)="save()" novalidate>
          <section class="grid">
            <app-field for="name" label="Name" [error]="fieldError('name')">
              <input id="name" name="name" required [(ngModel)]="name" />
            </app-field>

            <app-field for="country" label="Country" [error]="fieldError('country')">
              <input id="country" name="country" required [(ngModel)]="country" />
            </app-field>

            <app-field for="type" label="Type">
              <select id="type" name="type" [(ngModel)]="type">
                @for (option of types; track option.value) {
                  <option [value]="option.value">{{ option.label }}</option>
                }
              </select>
            </app-field>

            @if (type() === 'league') {
              <app-field
                for="tier"
                label="Tier"
                [optional]="true"
                hint="1 is the top division."
                [error]="fieldError('tier')"
              >
                <input id="tier" name="tier" type="number" min="1" [(ngModel)]="tier" />
              </app-field>
            }
          </section>

          @if (saveError()) {
            <app-error-state [message]="saveError()!" [requestId]="saveRequestId()" />
          }

          <footer class="actions">
            <button class="btn primary" type="submit" [disabled]="busy()">
              {{ busy() ? 'Saving…' : isEdit() ? 'Save changes' : 'Create competition' }}
            </button>
            <a class="btn" [routerLink]="cancelTarget()">Cancel</a>
          </footer>
        </form>
      }
    </main>
  `,
  styles: `
    .form-page { max-width: 38rem; padding-block: var(--space-6) var(--space-8); }
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
    .actions { display: flex; gap: var(--space-3); }
  `,
})
export class CompetitionForm {
  private readonly reader = inject(LEAGUE_READER);
  private readonly writer = inject(LEAGUE_WRITER);
  private readonly router = inject(Router);

  readonly id = input<string | undefined>(undefined);

  protected readonly types = TYPES;
  protected readonly name = signal('');
  protected readonly country = signal('');
  protected readonly type = signal<CompetitionType>('league');
  protected readonly tier = signal<number | null>(null);

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

  protected readonly loading = computed(() => this.isEdit() && this.existing.isLoading());
  protected readonly loadErrorMessage = computed(() =>
    messageFor(this.existing.error(), 'Could not load the competition.'),
  );

  constructor() {
    effect(() => {
      const league = this.existing.value();
      if (!league) return;
      this.name.set(league.name);
      this.country.set(league.country);
      this.type.set(league.competition_type);
      this.tier.set(league.tier ?? null);
    });
  }

  protected cancelTarget(): string {
    return this.id() ? `/competitions/${this.id()}` : '/competitions';
  }

  protected fieldError(field: string): string | null {
    return this.errors()[field] ?? null;
  }

  protected async save(): Promise<void> {
    if (this.busy()) return;

    const errors: Record<string, string> = {};
    const name = this.name().trim();
    const country = this.country().trim();

    if (!name) errors['name'] = 'A name is required.';
    if (!country) errors['country'] = 'A country is required.';
    if (this.tier() !== null && this.tier()! < 1) errors['tier'] = 'Must be 1 or greater.';

    this.errors.set(errors);
    if (Object.keys(errors).length > 0) return;

    const body: Partial<League> = {
      name,
      country,
      competition_type: this.type(),
      // A tier only travels with a league; a cup keeps whatever it had, which
      // is nothing.
      tier: this.type() === 'league' ? (this.tier() ?? undefined) : undefined,
    };

    this.busy.set(true);
    this.saveError.set(null);
    this.saveRequestId.set(null);

    try {
      const id = this.id();
      const saved = id ? await this.writer.update(id, body) : await this.writer.create(body);
      await this.router.navigate(['/competitions', saved.id]);
    } catch (error) {
      this.saveError.set(messageFor(error));
      this.saveRequestId.set(requestIdFor(error));
    } finally {
      this.busy.set(false);
    }
  }
}
