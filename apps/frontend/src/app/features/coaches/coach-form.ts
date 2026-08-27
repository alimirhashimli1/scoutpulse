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
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';

import { COACH_READER, COACH_WRITER } from '../../core/api/contracts';
import { LookupStore } from '../../core/api/lookup-store';
import { Permissions } from '../../core/auth/permissions';
import { Coach } from '../../core/models/football';
import { ClubSelect } from '../../shared/forms/club-select';
import { Field } from '../../shared/forms/field';
import { messageFor, requestIdFor } from '../../shared/forms/submit';
import { toApiDate, toDateInput } from '../../shared/util/dates';
import { ErrorState, Loading } from '../../shared/ui/states';

/**
 * Create or edit a coach.
 *
 * The same create/edit asymmetry as the player form, for the same reason: a
 * coach's club is derived from their spells, so `UpdateCoach` restores
 * `team_id` from the stored row before saving. The club is offered when
 * creating — it seeds the record — and absent when editing, where appointing
 * someone means recording a spell.
 */
@Component({
  selector: 'app-coach-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, Field, ClubSelect, Loading, ErrorState],
  template: `
    <main class="page form-page">
      @if (loading()) {
        <app-loading message="Loading coach…" />
      } @else if (existing.error()) {
        <app-error-state [message]="loadErrorMessage()" />
      } @else {
        <header class="head">
          <p class="eyebrow">{{ isEdit() ? 'Edit coach' : 'New coach' }}</p>
          <h1>{{ isEdit() ? name() || 'Coach' : 'Add a coach' }}</h1>
        </header>

        <form (ngSubmit)="save()" novalidate>
          <section class="grid">
            <app-field for="name" label="Name" [error]="fieldError('name')">
              <input id="name" name="name" required [(ngModel)]="name" />
            </app-field>

            @if (!isEdit()) {
              <app-field for="team" label="Club" [hint]="clubHint()" [optional]="true">
                <app-club-select
                  id="team"
                  [(value)]="teamId"
                  [restrict]="true"
                  [allowNone]="permissions.isAdmin()"
                  noneLabel="— unattached —"
                />
              </app-field>
            }

            <app-field for="first" label="First name" [optional]="true">
              <input id="first" name="first" [(ngModel)]="firstName" />
            </app-field>

            <app-field for="last" label="Last name" [optional]="true">
              <input id="last" name="last" [(ngModel)]="lastName" />
            </app-field>

            <app-field for="dob" label="Date of birth" [optional]="true">
              <input id="dob" name="dob" type="date" [max]="today" [(ngModel)]="dateOfBirth" />
            </app-field>

            <app-field for="nationality" label="Nationality" [optional]="true">
              <input id="nationality" name="nationality" [(ngModel)]="nationality" />
            </app-field>
          </section>

          @if (saveError()) {
            <app-error-state [message]="saveError()!" [requestId]="saveRequestId()" />
          }

          <footer class="actions">
            <button class="btn primary" type="submit" [disabled]="busy()">
              {{ busy() ? 'Saving…' : isEdit() ? 'Save changes' : 'Create coach' }}
            </button>
            <a class="btn" [routerLink]="cancelTarget()">Cancel</a>
          </footer>
        </form>
      }
    </main>
  `,
  styles: `
    .form-page {
      max-width: 40rem;
      padding-block: var(--space-6) var(--space-8);
    }
    .head {
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
      font-size: var(--text-2xl);
    }
    form {
      display: flex;
      flex-direction: column;
      gap: var(--space-5);
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
      gap: var(--space-4);
    }
    .actions {
      display: flex;
      gap: var(--space-3);
    }
    @media (max-width: 30rem) {
      .actions {
        flex-direction: column;
      }
      .actions .btn {
        justify-content: center;
      }
    }
  `,
})
export class CoachForm {
  private readonly reader = inject(COACH_READER);
  private readonly writer = inject(COACH_WRITER);
  private readonly lookup = inject(LookupStore);
  private readonly router = inject(Router);
  protected readonly permissions = inject(Permissions);

  readonly id = input<string | undefined>(undefined);

  protected readonly today = toDateInput(new Date().toISOString());

  protected readonly name = signal('');
  protected readonly teamId = signal<string | null>(null);
  protected readonly firstName = signal('');
  protected readonly lastName = signal('');
  protected readonly dateOfBirth = signal('');
  protected readonly nationality = signal('');

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
    messageFor(this.existing.error(), 'Could not load the coach.'),
  );

  constructor() {
    void this.lookup.loadTeams();

    effect(() => {
      const coach = this.existing.value();
      if (coach) this.fill(coach);
    });
  }

  protected clubHint(): string {
    return this.permissions.isAdmin()
      ? 'Their club now. Appointments over time are recorded as spells.'
      : 'Only clubs you may edit are listed. An unattached coach is administrator-only.';
  }

  protected cancelTarget(): string {
    return this.id() ? `/coaches/${this.id()}` : '/clubs';
  }

  protected fieldError(field: string): string | null {
    return this.errors()[field] ?? null;
  }

  protected async save(): Promise<void> {
    if (this.busy()) return;

    const name = this.name().trim();
    if (!name) {
      this.errors.set({ name: 'A name is required.' });
      return;
    }
    this.errors.set({});

    const body: Partial<Coach> = {
      name,
      first_name: blankToUndefined(this.firstName()),
      last_name: blankToUndefined(this.lastName()),
      date_of_birth: toApiDate(this.dateOfBirth()),
      nationality: blankToUndefined(this.nationality()),
    };
    // Create-only, for the same reason as the player form: an update would
    // accept it and then restore the stored value.
    if (!this.isEdit()) body.team_id = this.teamId();

    this.busy.set(true);
    this.saveError.set(null);
    this.saveRequestId.set(null);

    try {
      const id = this.id();
      const saved = id ? await this.writer.update(id, body) : await this.writer.create(body);
      await this.router.navigate(['/coaches', saved.id]);
    } catch (error) {
      this.saveError.set(messageFor(error));
      this.saveRequestId.set(requestIdFor(error));
    } finally {
      this.busy.set(false);
    }
  }

  private fill(coach: Coach): void {
    this.name.set(coach.name);
    this.firstName.set(coach.first_name ?? '');
    this.lastName.set(coach.last_name ?? '');
    this.dateOfBirth.set(toDateInput(coach.date_of_birth));
    this.nationality.set(coach.nationality ?? '');
  }
}

function blankToUndefined(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed === '' ? undefined : trimmed;
}
