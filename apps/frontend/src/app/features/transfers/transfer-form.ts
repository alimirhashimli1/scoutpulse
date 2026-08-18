import { ChangeDetectionStrategy, Component, computed, effect, inject, input, resource, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';

import {
  PLAYER_READER,
  SEASON_READER,
  TRANSFER_READER,
  TRANSFER_WRITER,
} from '../../core/api/contracts';
import { LookupStore } from '../../core/api/lookup-store';
import { Permissions } from '../../core/auth/permissions';
import { Transfer, TransferType } from '../../core/models/football';
import { ClubSelect } from '../../shared/forms/club-select';
import { Field } from '../../shared/forms/field';
import { messageFor, requestIdFor } from '../../shared/forms/submit';
import { MoneyPipe } from '../../shared/pipes/money-pipe';
import { toApiDate, todayInput } from '../../shared/util/dates';
import { MoneyParseError, formatMoneyInput, parseMoney } from '../../shared/util/money-input';
import { ErrorState, Loading } from '../../shared/ui/states';

const TRANSFER_TYPES: { value: TransferType; label: string; note?: string }[] = [
  { value: 'permanent', label: 'Permanent' },
  { value: 'loan', label: 'Loan' },
  { value: 'loan_return', label: 'Return from loan' },
  { value: 'free', label: 'Free transfer', note: 'Known to cost nothing.' },
  { value: 'youth_promotion', label: 'Youth promotion' },
  { value: 'released', label: 'Released' },
  { value: 'retired', label: 'Retired' },
];

const ENDS_CAREER_AT_CLUB = new Set<TransferType>(['released', 'retired']);

/**
 * Record a move. This is the only way a player changes club.
 *
 * The one design decision worth explaining is what the form **does not** ask.
 *
 * The API requires `from_team_id` to equal the player's current club exactly —
 * `checkOrigin` rejects anything else with "from_team_id does not match the
 * player's current club", and requires it be *omitted* for a free agent. A
 * selling-club dropdown would therefore have exactly one correct answer, and
 * every other choice would be a round trip that fails. So the origin is shown
 * as a fact and sent from the loaded player, and the form asks only where they
 * are going.
 *
 * That constraint is not arbitrary. The current squad is derived from transfer
 * history, so a move whose origin disagrees with the present would make the
 * two contradict each other permanently.
 *
 * Either club's editor may file the move — releasing a player and signing one
 * are the same event — which is why the submit button unlocks as soon as a
 * destination the user holds is chosen, even when they do not hold the seller.
 */
@Component({
  selector: 'app-transfer-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, Field, ClubSelect, Loading, ErrorState],
  template: `
    <main class="page form-page">
      @if (player.isLoading()) {
        <app-loading message="Loading player…" />
      } @else if (player.error()) {
        <app-error-state [message]="loadErrorMessage()" />
      } @else if (player.value(); as p) {
        <header class="head">
          <p class="eyebrow">Record a transfer</p>
          <h1>{{ p.name }}</h1>
          <p class="origin">
            Currently
            @if (p.team_id) {
              <strong>{{ clubName(p.team_id) }}</strong>
            } @else {
              <strong>a free agent</strong>
            }
            <!--
              Stated, not editable. The API requires the origin to match the
              player's present club, so there is only one valid value and
              offering a choice would only manufacture failures.
            -->
          </p>
        </header>

        <form (ngSubmit)="save()" novalidate>
          <section class="grid">
            <app-field for="type" label="Type" [error]="fieldError('transfer_type')">
              <select id="type" name="type" [(ngModel)]="type" (ngModelChange)="typeChanged()">
                @for (option of types; track option.value) {
                  <option [value]="option.value">{{ option.label }}</option>
                }
              </select>
            </app-field>

            <app-field
              for="date"
              label="Date"
              [error]="fieldError('transfer_date')"
              hint="When the move takes effect."
            >
              <input id="date" name="date" type="date" [max]="latestDate" [(ngModel)]="date" />
            </app-field>

            <app-field
              for="to"
              label="Joining"
              [hint]="destinationHint()"
              [error]="fieldError('to_team_id')"
            >
              <app-club-select
                id="to"
                [(value)]="toTeamId"
                [allowNone]="true"
                noneLabel="— leaving the dataset —"
              />
            </app-field>

            <app-field
              for="fee"
              label="Fee"
              [optional]="true"
              [hint]="feeHint()"
              [error]="fieldError('fee_minor')"
            >
              <input
                id="fee"
                name="fee"
                inputmode="decimal"
                [disabled]="isFree()"
                [(ngModel)]="fee"
              />
            </app-field>

            <app-field
              for="season"
              label="Season"
              [optional]="true"
              hint="Left blank, the season containing the date is attached."
            >
              <select id="season" name="season" [(ngModel)]="seasonId">
                <option value="">— from the date —</option>
                @for (season of seasons.value()?.items ?? []; track season.id) {
                  <option [value]="season.id">{{ season.label }}</option>
                }
              </select>
            </app-field>
          </section>

          @if (!permitted()) {
            <p class="notice">
              You may record this move only through a club you edit. Choose a destination you
              manage, or ask an administrator.
            </p>
          }

          @if (saveError()) {
            <app-error-state [message]="saveError()!" [requestId]="saveRequestId()" />
          }

          <footer class="actions">
            <button class="btn primary" type="submit" [disabled]="busy() || !permitted()">
              {{ busy() ? 'Recording…' : 'Record transfer' }}
            </button>
            <a class="btn" [routerLink]="['/players', playerId()]">Cancel</a>
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
    @media (max-width: 30rem) {
      .actions { flex-direction: column; }
      .actions .btn { justify-content: center; }
    }
  `,
})
export class TransferForm {
  private readonly players = inject(PLAYER_READER);
  private readonly seasonReader = inject(SEASON_READER);
  private readonly writer = inject(TRANSFER_WRITER);
  private readonly lookup = inject(LookupStore);
  private readonly permissions = inject(Permissions);
  private readonly router = inject(Router);

  /** The player being moved, from `/players/:id/transfer`. */
  readonly id = input.required<string>();

  protected readonly types = TRANSFER_TYPES;
  protected readonly playerId = computed(() => this.id());

  /** The API refuses a date more than a year out. */
  protected readonly latestDate = oneYearFromToday();

  protected readonly type = signal<TransferType>('permanent');
  protected readonly date = signal(todayInput());
  protected readonly toTeamId = signal<string | null>(null);
  protected readonly fee = signal('');
  protected readonly seasonId = signal('');

  protected readonly busy = signal(false);
  protected readonly saveError = signal<string | null>(null);
  protected readonly saveRequestId = signal<string | null>(null);
  private readonly errors = signal<Record<string, string>>({});

  protected readonly player = resource({
    params: () => ({ id: this.id() }),
    loader: async ({ params }) => {
      await this.lookup.loadTeams();
      return this.players.byId(params.id);
    },
  });

  protected readonly seasons = resource({
    loader: () => this.seasonReader.list({ limit: 50 }),
  });

  protected readonly isFree = computed(() => this.type() === 'free');

  /**
   * Whether the signed-in user may file *this* move.
   *
   * Recomputed as the destination changes, because the answer genuinely
   * depends on it: an editor holding only the buying club is refused until
   * they select it, and permitted the moment they do.
   */
  protected readonly permitted = computed(() =>
    this.permissions.canMoveBetween(this.player.value()?.team_id ?? null, this.toTeamId()),
  );

  protected readonly loadErrorMessage = computed(() =>
    messageFor(this.player.error(), 'Could not load the player.'),
  );

  constructor() {
    // A free transfer is defined as costing nothing, so anything typed in the
    // fee box would be rejected. Clearing it is less confusing than sending it
    // and reporting an error about a field that is now disabled.
    effect(() => {
      if (this.isFree()) this.fee.set('');
    });
  }

  protected clubName(id: string | null): string {
    return this.lookup.teamName(id, 'an unknown club');
  }

  protected destinationHint(): string {
    return ENDS_CAREER_AT_CLUB.has(this.type())
      ? 'A released or retired player joins nobody — leave this empty.'
      : 'Leave empty if they are leaving the dataset entirely.';
  }

  protected feeHint(): string {
    if (this.isFree()) return 'A free transfer carries no fee by definition.';
    return 'In euros. Blank means undisclosed, which is not the same as free.';
  }

  protected typeChanged(): void {
    // Releasing or retiring means there is no destination. Cleared rather than
    // disabled: the API permits the combination, so this is a sensible default
    // and not a rule the client invented.
    if (ENDS_CAREER_AT_CLUB.has(this.type())) this.toTeamId.set(null);
  }

  protected fieldError(field: string): string | null {
    return this.errors()[field] ?? null;
  }

  protected async save(): Promise<void> {
    if (this.busy()) return;

    const current = this.player.value();
    if (!current) return;

    const body = this.build(current.team_id);
    if (!body) return;

    this.busy.set(true);
    this.saveError.set(null);
    this.saveRequestId.set(null);

    try {
      await this.writer.record(body);
      // Straight to the player, where the new move is now the top of the
      // career table and the club in the header has changed.
      await this.router.navigate(['/players', this.id()]);
    } catch (error) {
      this.saveError.set(messageFor(error, 'Could not record the transfer.'));
      this.saveRequestId.set(requestIdFor(error));
    } finally {
      this.busy.set(false);
    }
  }

  private build(currentTeam: string | null): Partial<Transfer> | null {
    const errors: Record<string, string> = {};

    if (!this.date()) {
      errors['transfer_date'] = 'A date is required.';
    } else if (this.date() > this.latestDate) {
      errors['transfer_date'] = 'No more than a year ahead.';
    }

    // Mirrors validateTransfer: a move with neither end says nothing at all.
    if (!currentTeam && !this.toTeamId()) {
      errors['to_team_id'] = 'A free agent must be joining someone.';
    }
    if (currentTeam && this.toTeamId() === currentTeam) {
      errors['to_team_id'] = 'The two clubs must differ.';
    }

    let fee: number | null = null;
    if (!this.isFree() && this.fee().trim()) {
      try {
        fee = parseMoney(this.fee());
      } catch (error) {
        errors['fee_minor'] = error instanceof MoneyParseError ? error.message : 'Not a valid amount.';
      }
    }

    this.errors.set(errors);
    if (Object.keys(errors).length > 0) return null;

    return {
      player_id: this.id(),
      // Sent from the loaded player, never from an input. Omitted entirely for
      // a free agent, which is what the API requires.
      from_team_id: currentTeam,
      to_team_id: this.toTeamId(),
      transfer_date: toApiDate(this.date()),
      transfer_type: this.type(),
      fee_minor: fee,
      season_id: this.seasonId() || null,
    };
  }
}

/**
 * Correct a recorded move.
 *
 * A deliberately smaller form than recording one, because the API allows far
 * less: `UpdateTransfer` overwrites the player, both clubs and the date from
 * the stored row before saving, so only type, fee, currency and season can
 * actually change. A client that round-trips a GET into a PUT therefore cannot
 * move anyone by accident — and a form offering those fields would be lying
 * about what it does.
 *
 * The immutable facts are shown as context so the person correcting a fee can
 * see which move they are looking at.
 */
@Component({
  selector: 'app-transfer-edit',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, DatePipe, MoneyPipe, Field, Loading, ErrorState],
  template: `
    <main class="page form-page">
      @if (transfer.isLoading()) {
        <app-loading message="Loading transfer…" />
      } @else if (transfer.error()) {
        <app-error-state [message]="loadErrorMessage()" />
      } @else if (transfer.value(); as t) {
        <header class="head">
          <p class="eyebrow">Correct a transfer</p>
          <h1>{{ clubName(t.from_team_id) }} → {{ clubName(t.to_team_id) }}</h1>
          <dl class="fixed">
            <div>
              <dt>Date</dt>
              <dd>{{ t.transfer_date | date: 'd MMM y' }}</dd>
            </div>
            <div>
              <dt>Recorded fee</dt>
              <dd class="tabular">{{ t.fee_minor | money: { currency: t.currency } }}</dd>
            </div>
          </dl>
          <p class="note">
            The player, both clubs and the date cannot be changed here — they are what the current
            squad is derived from. Delete the record instead if it is wrong.
          </p>
        </header>

        <form (ngSubmit)="save()" novalidate>
          <section class="grid">
            <app-field for="type" label="Type">
              <select id="type" name="type" [(ngModel)]="type">
                @for (option of types; track option.value) {
                  <option [value]="option.value">{{ option.label }}</option>
                }
              </select>
            </app-field>

            <app-field for="fee" label="Fee" [optional]="true" [hint]="feeHint()" [error]="feeError()">
              <input id="fee" name="fee" inputmode="decimal" [disabled]="isFree()" [(ngModel)]="fee" />
            </app-field>
          </section>

          @if (saveError()) {
            <app-error-state [message]="saveError()!" [requestId]="saveRequestId()" />
          }

          <footer class="actions">
            <button class="btn primary" type="submit" [disabled]="busy()">
              {{ busy() ? 'Saving…' : 'Save correction' }}
            </button>
            <a class="btn" routerLink="/transfers">Cancel</a>
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
    h1 { font-size: var(--text-xl); margin-bottom: var(--space-4); }
    .fixed { display: flex; gap: var(--space-6); margin-bottom: var(--space-4); }
    dt { font-size: var(--text-xs); text-transform: uppercase; letter-spacing: 0.08em; color: var(--muted); }
    dd { margin: 0; font-weight: 600; }
    .note {
      font-size: var(--text-sm);
      color: var(--muted);
      border-left: 2px solid var(--line);
      padding-left: var(--space-3);
    }
    form { display: flex; flex-direction: column; gap: var(--space-5); }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
      gap: var(--space-4);
    }
    .actions { display: flex; gap: var(--space-3); }
  `,
})
export class TransferEdit {
  private readonly reader = inject(TRANSFER_READER);
  private readonly writer = inject(TRANSFER_WRITER);
  private readonly lookup = inject(LookupStore);
  private readonly router = inject(Router);

  readonly id = input.required<string>();

  protected readonly types = TRANSFER_TYPES;
  protected readonly type = signal<TransferType>('permanent');
  protected readonly fee = signal('');
  protected readonly busy = signal(false);
  protected readonly saveError = signal<string | null>(null);
  protected readonly saveRequestId = signal<string | null>(null);
  protected readonly feeError = signal<string | null>(null);

  protected readonly transfer = resource({
    params: () => ({ id: this.id() }),
    loader: async ({ params }) => {
      await this.lookup.loadTeams();
      return this.reader.byId(params.id);
    },
  });

  protected readonly isFree = computed(() => this.type() === 'free');

  protected readonly loadErrorMessage = computed(() =>
    messageFor(this.transfer.error(), 'Could not load the transfer.'),
  );

  constructor() {
    effect(() => {
      const t = this.transfer.value();
      if (!t) return;
      this.type.set(t.transfer_type);
      this.fee.set(formatMoneyInput(t.fee_minor));
    });
  }

  protected clubName(id: string | null): string {
    return this.lookup.teamName(id, '—');
  }

  protected feeHint(): string {
    return this.isFree()
      ? 'A free transfer carries no fee by definition.'
      : 'Blank means undisclosed, which is not the same as free.';
  }

  protected async save(): Promise<void> {
    if (this.busy()) return;

    const existing = this.transfer.value();
    if (!existing) return;

    let fee: number | null = null;
    if (!this.isFree() && this.fee().trim()) {
      try {
        fee = parseMoney(this.fee());
      } catch (error) {
        this.feeError.set(error instanceof MoneyParseError ? error.message : 'Not a valid amount.');
        return;
      }
    }
    this.feeError.set(null);

    this.busy.set(true);
    this.saveError.set(null);
    this.saveRequestId.set(null);

    try {
      await this.writer.update(existing.id, {
        transfer_type: this.type(),
        fee_minor: fee,
        currency: existing.currency,
        season_id: existing.season_id,
      });
      await this.router.navigate(['/players', existing.player_id]);
    } catch (error) {
      this.saveError.set(messageFor(error, 'Could not save the correction.'));
      this.saveRequestId.set(requestIdFor(error));
    } finally {
      this.busy.set(false);
    }
  }
}

/**
 * The API's ceiling on a future-dated move.
 *
 * Through a Date rather than string arithmetic on the year, so 29 February
 * rolls to 1 March in the following year instead of producing a date that does
 * not exist.
 */
function oneYearFromToday(now: Date = new Date()): string {
  const limit = new Date(now);
  limit.setFullYear(limit.getFullYear() + 1);
  return todayInput(limit);
}
