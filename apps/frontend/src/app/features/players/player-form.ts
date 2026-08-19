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

import { PLAYER_READER, PLAYER_WRITER } from '../../core/api/contracts';
import { LookupStore } from '../../core/api/lookup-store';
import { Permissions } from '../../core/auth/permissions';
import { Player } from '../../core/models/football';
import { ClubSelect } from '../../shared/forms/club-select';
import { Field } from '../../shared/forms/field';
import { messageFor, requestIdFor } from '../../shared/forms/submit';
import { toApiDate, toDateInput } from '../../shared/util/dates';
import { MoneyParseError, parseMoney } from '../../shared/util/money-input';
import { ErrorState, Loading } from '../../shared/ui/states';

/** Suggestions, not a closed set — the API stores whatever string is sent. */
const POSITIONS = [
  'Goalkeeper',
  'Centre-Back',
  'Left-Back',
  'Right-Back',
  'Defensive Midfield',
  'Central Midfield',
  'Attacking Midfield',
  'Left Winger',
  'Right Winger',
  'Second Striker',
  'Centre-Forward',
];

/**
 * Create or edit a player.
 *
 * **The two modes are not the same form**, and the difference is forced by the
 * API rather than chosen:
 *
 * | Field | Create | Edit |
 * |---|---|---|
 * | Club | offered — it writes the opening transfer | **absent** |
 * | Market value | offered — it writes the opening valuation | **absent** |
 * | Everything else | offered | offered |
 *
 * `POST /players` takes a club and a value, and the repository records both as
 * history: an opening transfer *into* that club, and a first valuation. That is
 * what makes a newly created player appear correctly on a club page.
 *
 * `PUT /players/{id}` accepts the same two fields and **silently discards
 * them** — the service overwrites them from the stored row before saving,
 * because a player's club is derived from transfer history and their value
 * from the valuation series. Rendering either as an editable field would be a
 * control that appears to work and does nothing. Moving a player is a
 * transfer; revaluing one is a valuation. Both are separate forms.
 *
 * docs/FRONTEND.md §2.4 recorded the club half of this. The market value half
 * behaves identically and is recorded here.
 */
@Component({
  selector: 'app-player-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, Field, ClubSelect, Loading, ErrorState],
  template: `
    <main class="page form-page">
      @if (loading()) {
        <app-loading message="Loading player…" />
      } @else if (loadError()) {
        <app-error-state [message]="loadErrorMessage()" />
      } @else {
        <header class="head">
          <p class="eyebrow">{{ isEdit() ? 'Edit player' : 'New player' }}</p>
          <h1>{{ isEdit() ? name() || 'Player' : 'Add a player' }}</h1>
        </header>

        <form (ngSubmit)="save()" novalidate>
          <section class="grid">
            <app-field for="name" label="Name" [error]="fieldError('name')">
              <input id="name" name="name" required autocomplete="off" [(ngModel)]="name" />
            </app-field>

            <app-field
              for="position"
              label="Position"
              hint="Suggestions only — anything can be typed."
              [error]="fieldError('position')"
            >
              <input
                id="position"
                name="position"
                list="positions"
                required
                autocomplete="off"
                [(ngModel)]="position"
              />
              <datalist id="positions">
                @for (option of positions; track option) {
                  <option [value]="option"></option>
                }
              </datalist>
            </app-field>

            @if (!isEdit()) {
              <!--
                Create only. On edit this field would be accepted by the API
                and then thrown away, which is worse than not offering it.
              -->
              <app-field
                for="team"
                label="Club"
                [hint]="clubHint()"
                [error]="fieldError('team_id')"
              >
                <app-club-select
                  id="team"
                  [(value)]="teamId"
                  [restrict]="true"
                  [allowNone]="permissions.isAdmin()"
                  noneLabel="— free agent —"
                />
              </app-field>

              <app-field
                for="value"
                label="Market value"
                hint="In euros. Leave blank for none."
                [error]="fieldError('market_value_minor')"
                [optional]="true"
              >
                <input id="value" name="value" inputmode="decimal" [(ngModel)]="marketValue" />
              </app-field>
            }
          </section>

          <details [open]="isEdit()">
            <summary>Personal details</summary>
            <section class="grid">
              <app-field for="first" label="First name" [optional]="true">
                <input id="first" name="first" [(ngModel)]="firstName" />
              </app-field>

              <app-field for="last" label="Last name" [optional]="true">
                <input id="last" name="last" [(ngModel)]="lastName" />
              </app-field>

              <app-field
                for="dob"
                label="Date of birth"
                [optional]="true"
                [error]="fieldError('date_of_birth')"
              >
                <input id="dob" name="dob" type="date" [max]="today" [(ngModel)]="dateOfBirth" />
              </app-field>

              <app-field for="nationality" label="Nationality" [optional]="true">
                <input id="nationality" name="nationality" [(ngModel)]="nationality" />
              </app-field>

              <app-field for="second" label="Second nationality" [optional]="true">
                <input id="second" name="second" [(ngModel)]="secondNationality" />
              </app-field>

              <app-field
                for="height"
                label="Height (cm)"
                [optional]="true"
                [error]="fieldError('height_cm')"
              >
                <input
                  id="height"
                  name="height"
                  type="number"
                  min="100"
                  max="250"
                  [(ngModel)]="heightCm"
                />
              </app-field>

              <app-field for="foot" label="Preferred foot" [optional]="true">
                <select id="foot" name="foot" [(ngModel)]="preferredFoot">
                  <option value="">— unknown —</option>
                  <option value="right">Right</option>
                  <option value="left">Left</option>
                  <option value="both">Both</option>
                </select>
              </app-field>
            </section>
          </details>

          <details [open]="isEdit()">
            <summary>Club details</summary>
            <section class="grid">
              <app-field
                for="squad"
                label="Squad number"
                [optional]="true"
                [error]="fieldError('squad_number')"
              >
                <input
                  id="squad"
                  name="squad"
                  type="number"
                  min="1"
                  max="99"
                  [(ngModel)]="squadNumber"
                />
              </app-field>

              <app-field for="agent" label="Agent" [optional]="true">
                <input id="agent" name="agent" [(ngModel)]="agent" />
              </app-field>

              <app-field for="cstart" label="Contract from" [optional]="true">
                <input id="cstart" name="cstart" type="date" [(ngModel)]="contractStart" />
              </app-field>

              <app-field
                for="cuntil"
                label="Contract until"
                [optional]="true"
                [error]="fieldError('contract_until')"
              >
                <input id="cuntil" name="cuntil" type="date" [(ngModel)]="contractUntil" />
              </app-field>
            </section>
          </details>

          @if (saveError()) {
            <app-error-state [message]="saveError()!" [requestId]="saveRequestId()" />
          }

          <footer class="actions">
            <button class="btn primary" type="submit" [disabled]="busy()">
              {{ busy() ? 'Saving…' : isEdit() ? 'Save changes' : 'Create player' }}
            </button>
            <a class="btn" [routerLink]="cancelTarget()">Cancel</a>
          </footer>
        </form>
      }
    </main>
  `,
  styles: `
    .form-page {
      max-width: 44rem;
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
    details {
      border-top: 1px solid var(--line);
      padding-top: var(--space-4);
    }
    summary {
      cursor: pointer;
      font-weight: 600;
      margin-bottom: var(--space-4);
      color: var(--ink-soft);
    }
    .actions {
      display: flex;
      gap: var(--space-3);
      align-items: center;
    }
    @media (max-width: 30rem) {
      .actions {
        flex-direction: column;
        align-items: stretch;
      }
      .actions .btn {
        justify-content: center;
      }
    }
  `,
})
export class PlayerForm {
  private readonly reader = inject(PLAYER_READER);
  private readonly writer = inject(PLAYER_WRITER);
  private readonly lookup = inject(LookupStore);
  private readonly router = inject(Router);
  protected readonly permissions = inject(Permissions);

  /** Absent on `/players/new`, present on `/players/:id/edit`. */
  readonly id = input<string | undefined>(undefined);

  protected readonly positions = POSITIONS;
  protected readonly today = toDateInput(new Date().toISOString());

  protected readonly name = signal('');
  protected readonly position = signal('');
  protected readonly teamId = signal<string | null>(null);
  protected readonly marketValue = signal('');
  protected readonly firstName = signal('');
  protected readonly lastName = signal('');
  protected readonly dateOfBirth = signal('');
  protected readonly nationality = signal('');
  protected readonly secondNationality = signal('');
  protected readonly heightCm = signal<number | null>(null);
  protected readonly preferredFoot = signal('');
  protected readonly agent = signal('');
  protected readonly squadNumber = signal<number | null>(null);
  protected readonly contractStart = signal('');
  protected readonly contractUntil = signal('');

  protected readonly busy = signal(false);
  protected readonly saveError = signal<string | null>(null);
  protected readonly saveRequestId = signal<string | null>(null);
  private readonly errors = signal<Record<string, string>>({});

  protected readonly isEdit = computed(() => !!this.id());

  /** Only loads on edit. `resource` skips the loader when params are undefined. */
  private readonly existing = resource({
    params: () => {
      const id = this.id();
      return id ? { id } : undefined;
    },
    loader: async ({ params }) => {
      await this.lookup.loadTeams();
      return this.reader.byId(params.id);
    },
  });

  protected readonly loading = computed(() => this.isEdit() && this.existing.isLoading());
  protected readonly loadError = computed(() => this.existing.error());
  protected readonly loadErrorMessage = computed(() =>
    messageFor(this.existing.error(), 'Could not load the player.'),
  );

  constructor() {
    // Fill the form once the record lands. An effect rather than doing it in
    // the loader, so a failed save leaves what the user typed intact instead
    // of resetting it under them.
    effect(() => {
      const player = this.existing.value();
      if (player) this.fill(player);
    });

    // Clubs are needed before the select can render names.
    void this.lookup.loadTeams();
  }

  protected clubHint(): string {
    return this.permissions.isAdmin()
      ? 'Recorded as the opening transfer. Leave empty for a free agent.'
      : 'Only clubs you may edit are listed. Creating an unattached player is administrator-only.';
  }

  protected cancelTarget(): string {
    return this.id() ? `/players/${this.id()}` : '/';
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
      await this.router.navigate(['/players', saved.id]);
    } catch (error) {
      this.saveError.set(messageFor(error));
      this.saveRequestId.set(requestIdFor(error));
    } finally {
      this.busy.set(false);
    }
  }

  /**
   * Builds the payload, or returns null having set the field errors.
   *
   * These checks mirror `validatePlayer` in the service. They exist to answer
   * before a round trip, not instead of one — the server re-runs every one of
   * them, and anything it rejects that this missed still surfaces through
   * `saveError`.
   */
  private build(): Partial<Player> | null {
    const errors: Record<string, string> = {};

    const name = this.name().trim();
    if (!name) errors['name'] = 'A name is required.';

    const position = this.position().trim();
    if (!position) errors['position'] = 'A position is required.';

    const height = this.heightCm();
    if (height !== null && (height < 100 || height > 250)) {
      errors['height_cm'] = 'Between 100 and 250 cm.';
    }

    const squad = this.squadNumber();
    if (squad !== null && (squad < 1 || squad > 99)) {
      errors['squad_number'] = 'Between 1 and 99.';
    }

    if (this.dateOfBirth() && this.dateOfBirth() > this.today) {
      errors['date_of_birth'] = 'Cannot be in the future.';
    }

    if (
      this.contractStart() &&
      this.contractUntil() &&
      this.contractUntil() < this.contractStart()
    ) {
      errors['contract_until'] = 'Must be on or after the contract start.';
    }

    let value: number | null = null;
    if (!this.isEdit() && this.marketValue().trim()) {
      try {
        value = parseMoney(this.marketValue());
      } catch (error) {
        errors['market_value_minor'] =
          error instanceof MoneyParseError ? error.message : 'Not a valid amount.';
      }
    }

    this.errors.set(errors);
    if (Object.keys(errors).length > 0) return null;

    const body: Partial<Player> = {
      name,
      position,
      first_name: blankToUndefined(this.firstName()),
      last_name: blankToUndefined(this.lastName()),
      date_of_birth: toApiDate(this.dateOfBirth()),
      nationality: blankToUndefined(this.nationality()),
      second_nationality: blankToUndefined(this.secondNationality()),
      height_cm: this.heightCm() ?? undefined,
      preferred_foot: blankToUndefined(this.preferredFoot()) as Player['preferred_foot'],
      agent: blankToUndefined(this.agent()),
      squad_number: this.squadNumber() ?? undefined,
      contract_start: toApiDate(this.contractStart()),
      contract_until: toApiDate(this.contractUntil()),
    };

    // Club and value are create-only. Sending them on an update would be
    // harmless — the API discards both — but sending fields that are known to
    // be ignored invites someone to conclude they work.
    if (!this.isEdit()) {
      body.team_id = this.teamId();
      if (value !== null) body.market_value_minor = value;
    }

    return body;
  }

  private fill(player: Player): void {
    this.name.set(player.name);
    this.position.set(player.position);
    this.firstName.set(player.first_name ?? '');
    this.lastName.set(player.last_name ?? '');
    this.dateOfBirth.set(toDateInput(player.date_of_birth));
    this.nationality.set(player.nationality ?? '');
    this.secondNationality.set(player.second_nationality ?? '');
    this.heightCm.set(player.height_cm ?? null);
    this.preferredFoot.set(player.preferred_foot ?? '');
    this.agent.set(player.agent ?? '');
    this.squadNumber.set(player.squad_number ?? null);
    this.contractStart.set(toDateInput(player.contract_start));
    this.contractUntil.set(toDateInput(player.contract_until));
  }
}

/** An untouched optional text field must be omitted, not sent as "". */
function blankToUndefined(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed === '' ? undefined : trimmed;
}
