import { ChangeDetectionStrategy, Component, computed, inject, input, resource, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';

import { PLAYER_READER, PLAYER_WRITER } from '../../core/api/contracts';
import { MarketValue } from '../../core/models/football';
import { Field } from '../../shared/forms/field';
import { messageFor, requestIdFor } from '../../shared/forms/submit';
import { MoneyPipe } from '../../shared/pipes/money-pipe';
import { toApiDate, todayInput } from '../../shared/util/dates';
import { MoneyParseError, parseMoney } from '../../shared/util/money-input';
import { ErrorState, Loading } from '../../shared/ui/states';

/**
 * Record a market valuation. Administrators only.
 *
 * `RecordValue` calls `RequireAdmin` with no club exception, so a club's own
 * editor cannot value their players — which is the point. A valuation is an
 * assessment *of* a club's squad, and letting the club set it would make the
 * number worthless.
 *
 * The valuation series is the source of truth: the figure on the player's
 * profile follows the latest one rather than being stored independently. So
 * this form is the only way that number changes, and the chart on the profile
 * gains a point the moment it saves.
 */
@Component({
  selector: 'app-value-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, MoneyPipe, Field, Loading, ErrorState],
  template: `
    <main class="page form-page">
      @if (player.isLoading()) {
        <app-loading message="Loading player…" />
      } @else if (player.error()) {
        <app-error-state [message]="loadErrorMessage()" />
      } @else if (player.value(); as p) {
        <header class="head">
          <p class="eyebrow">Record a valuation</p>
          <h1>{{ p.name }}</h1>
          <p class="current">
            Currently valued at
            <strong class="tabular">{{
              p.market_value_minor | money: { currency: p.currency, compact: true }
            }}</strong>
          </p>
        </header>

        <form (ngSubmit)="save()" novalidate>
          <section class="grid">
            <app-field
              for="value"
              label="Value"
              hint="In euros. 25000000 or 25000000.00."
              [error]="fieldError('value_minor')"
            >
              <input id="value" name="value" inputmode="decimal" required [(ngModel)]="value" />
            </app-field>

            <app-field
              for="on"
              label="Valued on"
              hint="Cannot be in the future."
              [error]="fieldError('valued_on')"
            >
              <input id="on" name="on" type="date" [max]="today" [(ngModel)]="valuedOn" />
            </app-field>
          </section>

          @if (saveError()) {
            <app-error-state [message]="saveError()!" [requestId]="saveRequestId()" />
          }

          <footer class="actions">
            <button class="btn primary" type="submit" [disabled]="busy()">
              {{ busy() ? 'Recording…' : 'Record valuation' }}
            </button>
            <a class="btn" [routerLink]="['/players', id()]">Cancel</a>
          </footer>
        </form>
      }
    </main>
  `,
  styles: `
    .form-page { max-width: 34rem; padding-block: var(--space-6) var(--space-8); }
    .head { margin-bottom: var(--space-6); }
    .eyebrow {
      font-family: var(--font-mono); font-size: var(--text-xs);
      letter-spacing: 0.12em; text-transform: uppercase;
      color: var(--muted); margin-bottom: var(--space-2);
    }
    h1 { font-size: var(--text-2xl); margin-bottom: var(--space-2); }
    .current { color: var(--ink-soft); }
    form { display: flex; flex-direction: column; gap: var(--space-5); }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(13rem, 1fr));
      gap: var(--space-4);
    }
    .actions { display: flex; gap: var(--space-3); }
  `,
})
export class ValueForm {
  private readonly reader = inject(PLAYER_READER);
  private readonly writer = inject(PLAYER_WRITER);
  private readonly router = inject(Router);

  readonly id = input.required<string>();

  protected readonly today = todayInput();
  protected readonly value = signal('');
  protected readonly valuedOn = signal(todayInput());

  protected readonly busy = signal(false);
  protected readonly saveError = signal<string | null>(null);
  protected readonly saveRequestId = signal<string | null>(null);
  private readonly errors = signal<Record<string, string>>({});

  protected readonly player = resource({
    params: () => ({ id: this.id() }),
    loader: ({ params }) => this.reader.byId(params.id),
  });

  protected readonly loadErrorMessage = computed(() =>
    messageFor(this.player.error(), 'Could not load the player.'),
  );

  protected fieldError(field: string): string | null {
    return this.errors()[field] ?? null;
  }

  protected async save(): Promise<void> {
    if (this.busy()) return;

    const errors: Record<string, string> = {};
    let minor: number | null = null;

    try {
      minor = parseMoney(this.value());
      // Unlike a transfer fee, a valuation cannot be undisclosed — the record
      // exists to state a number.
      if (minor === null) errors['value_minor'] = 'A value is required.';
    } catch (error) {
      errors['value_minor'] = error instanceof MoneyParseError ? error.message : 'Not a valid amount.';
    }

    if (!this.valuedOn()) {
      errors['valued_on'] = 'A date is required.';
    } else if (this.valuedOn() > this.today) {
      errors['valued_on'] = 'Cannot be in the future.';
    }

    this.errors.set(errors);
    if (Object.keys(errors).length > 0 || minor === null) return;

    const body: Partial<MarketValue> = {
      player_id: this.id(),
      value_minor: minor,
      valued_on: toApiDate(this.valuedOn()),
    };

    this.busy.set(true);
    this.saveError.set(null);
    this.saveRequestId.set(null);

    try {
      await this.writer.recordValue(this.id(), body);
      await this.router.navigate(['/players', this.id()]);
    } catch (error) {
      this.saveError.set(messageFor(error, 'Could not record the valuation.'));
      this.saveRequestId.set(requestIdFor(error));
    } finally {
      this.busy.set(false);
    }
  }
}
