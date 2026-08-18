import { ChangeDetectionStrategy, Component, computed, inject, input, resource, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';

import { COACH_READER, COACH_WRITER } from '../../core/api/contracts';
import { LookupStore } from '../../core/api/lookup-store';
import { Permissions } from '../../core/auth/permissions';
import { CoachSpell, SpellRole } from '../../core/models/football';
import { ClubSelect } from '../../shared/forms/club-select';
import { Field } from '../../shared/forms/field';
import { messageFor, requestIdFor } from '../../shared/forms/submit';
import { toApiDate, todayInput } from '../../shared/util/dates';
import { ErrorState, Loading } from '../../shared/ui/states';

const ROLES: { value: SpellRole; label: string }[] = [
  { value: 'head_coach', label: 'Head coach' },
  { value: 'assistant_coach', label: 'Assistant coach' },
  { value: 'interim_coach', label: 'Interim coach' },
  { value: 'caretaker', label: 'Caretaker' },
  { value: 'director_of_football', label: 'Director of football' },
  { value: 'youth_coach', label: 'Youth coach' },
];

/**
 * Record an appointment — a coach's spell at a club.
 *
 * The counterpart of a transfer: it is how a coach's club changes, and how the
 * career on their profile is built. Leaving the end date empty means the spell
 * is open, which is what makes it their *current* club.
 *
 * Authorisation matches transfers: `RequireEitherTeam(coach's club, the new
 * club)`, so the club losing a coach and the club hiring one can both file it.
 */
@Component({
  selector: 'app-spell-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, Field, ClubSelect, Loading, ErrorState],
  template: `
    <main class="page form-page">
      @if (coach.isLoading()) {
        <app-loading message="Loading coach…" />
      } @else if (coach.error()) {
        <app-error-state [message]="loadErrorMessage()" />
      } @else if (coach.value(); as c) {
        <header class="head">
          <p class="eyebrow">Record an appointment</p>
          <h1>{{ c.name }}</h1>
          <p class="origin">
            Currently
            @if (c.team_id) {
              <strong>{{ clubName(c.team_id) }}</strong>
            } @else {
              <strong>unattached</strong>
            }
          </p>
        </header>

        <form (ngSubmit)="save()" novalidate>
          <section class="grid">
            <app-field for="club" label="Club" [error]="fieldError('team_id')">
              <app-club-select id="club" [(value)]="teamId" [allowNone]="true" noneLabel="— none —" />
            </app-field>

            <app-field for="role" label="Role">
              <select id="role" name="role" [(ngModel)]="role">
                @for (option of roles; track option.value) {
                  <option [value]="option.value">{{ option.label }}</option>
                }
              </select>
            </app-field>

            <app-field for="start" label="From" [error]="fieldError('start_date')">
              <input id="start" name="start" type="date" [max]="latestStart" [(ngModel)]="startDate" />
            </app-field>

            <app-field
              for="end"
              label="Until"
              [optional]="true"
              hint="Leave empty while the spell is ongoing."
              [error]="fieldError('end_date')"
            >
              <input id="end" name="end" type="date" [(ngModel)]="endDate" />
            </app-field>
          </section>

          @if (!permitted()) {
            <p class="notice">
              You may record this only through a club you edit. Choose a club you manage, or ask an
              administrator.
            </p>
          }

          @if (saveError()) {
            <app-error-state [message]="saveError()!" [requestId]="saveRequestId()" />
          }

          <footer class="actions">
            <button class="btn primary" type="submit" [disabled]="busy() || !permitted()">
              {{ busy() ? 'Recording…' : 'Record appointment' }}
            </button>
            <a class="btn" [routerLink]="['/coaches', id()]">Cancel</a>
          </footer>
        </form>
      }
    </main>
  `,
  styles: `
    .form-page { max-width: 40rem; padding-block: var(--space-6) var(--space-8); }
    .head { margin-bottom: var(--space-6); }
    .eyebrow {
      font-family: var(--font-mono); font-size: var(--text-xs);
      letter-spacing: 0.12em; text-transform: uppercase;
      color: var(--muted); margin-bottom: var(--space-2);
    }
    h1 { font-size: var(--text-2xl); margin-bottom: var(--space-2); }
    .origin { color: var(--ink-soft); }
    form { display: flex; flex-direction: column; gap: var(--space-5); }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
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
export class SpellForm {
  private readonly reader = inject(COACH_READER);
  private readonly writer = inject(COACH_WRITER);
  private readonly lookup = inject(LookupStore);
  private readonly permissions = inject(Permissions);
  private readonly router = inject(Router);

  readonly id = input.required<string>();

  protected readonly roles = ROLES;
  /** The API refuses a spell starting more than a year out. */
  protected readonly latestStart = oneYearFromToday();

  protected readonly teamId = signal<string | null>(null);
  protected readonly role = signal<SpellRole>('head_coach');
  protected readonly startDate = signal(todayInput());
  protected readonly endDate = signal('');

  protected readonly busy = signal(false);
  protected readonly saveError = signal<string | null>(null);
  protected readonly saveRequestId = signal<string | null>(null);
  private readonly errors = signal<Record<string, string>>({});

  protected readonly coach = resource({
    params: () => ({ id: this.id() }),
    loader: async ({ params }) => {
      await this.lookup.loadTeams();
      return this.reader.byId(params.id);
    },
  });

  protected readonly permitted = computed(() =>
    this.permissions.canMoveBetween(this.coach.value()?.team_id ?? null, this.teamId()),
  );

  protected readonly loadErrorMessage = computed(() =>
    messageFor(this.coach.error(), 'Could not load the coach.'),
  );

  protected clubName(id: string | null): string {
    return this.lookup.teamName(id, 'an unknown club');
  }

  protected fieldError(field: string): string | null {
    return this.errors()[field] ?? null;
  }

  protected async save(): Promise<void> {
    if (this.busy()) return;

    const errors: Record<string, string> = {};

    if (!this.startDate()) {
      errors['start_date'] = 'A start date is required.';
    } else if (this.startDate() > this.latestStart) {
      errors['start_date'] = 'No more than a year ahead.';
    }

    if (this.endDate() && this.endDate() < this.startDate()) {
      errors['end_date'] = 'Must be on or after the start.';
    }

    this.errors.set(errors);
    if (Object.keys(errors).length > 0) return;

    const spell: Partial<CoachSpell> = {
      coach_id: this.id(),
      team_id: this.teamId(),
      role: this.role(),
      start_date: toApiDate(this.startDate()),
      end_date: toApiDate(this.endDate()) ?? null,
    };

    this.busy.set(true);
    this.saveError.set(null);
    this.saveRequestId.set(null);

    try {
      await this.writer.recordSpell(this.id(), spell);
      await this.router.navigate(['/coaches', this.id()]);
    } catch (error) {
      this.saveError.set(messageFor(error, 'Could not record the appointment.'));
      this.saveRequestId.set(requestIdFor(error));
    } finally {
      this.busy.set(false);
    }
  }
}

function oneYearFromToday(now: Date = new Date()): string {
  const limit = new Date(now);
  limit.setFullYear(limit.getFullYear() + 1);
  return todayInput(limit);
}
