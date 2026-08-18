import { ChangeDetectionStrategy, Component, computed, inject, resource, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { RouterLink } from '@angular/router';

import { ApiError } from '../../core/api/api-error';
import { TRANSFER_READER } from '../../core/api/contracts';
import { LookupStore } from '../../core/api/lookup-store';
import { PageQuery } from '../../core/api/page';
import { Transfer, TransferType } from '../../core/models/football';
import { MoneyPipe } from '../../shared/pipes/money-pipe';
import { Paginator } from '../../shared/pagination/paginator';
import { Empty, ErrorState, Loading } from '../../shared/ui/states';

const TYPES: { value: TransferType | ''; label: string }[] = [
  { value: '', label: 'All moves' },
  { value: 'permanent', label: 'Permanent' },
  { value: 'loan', label: 'Loan' },
  { value: 'loan_return', label: 'Loan return' },
  { value: 'free', label: 'Free' },
  { value: 'youth_promotion', label: 'Youth' },
  { value: 'released', label: 'Released' },
  { value: 'retired', label: 'Retired' },
];

/**
 * The transfer feed — the centre of the product.
 *
 * Club names come from LookupStore rather than the transfer rows, which carry
 * ids only. See that file for why, and for where the approach stops scaling.
 */
@Component({
  selector: 'app-transfer-feed',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, DatePipe, MoneyPipe, Paginator, Loading, Empty, ErrorState],
  template: `
    <main class="page feed">
      <header>
        <h1>Transfers</h1>
        <p class="standfirst">Every recorded move, newest first.</p>
      </header>

      <nav class="filters">
        @for (option of types; track option.value) {
          <button
            type="button"
            [class.active]="type() === option.value"
            (click)="setType(option.value)"
          >{{ option.label }}</button>
        }
      </nav>

      @if (transfers.isLoading()) {
        <app-loading message="Loading transfers…" />
      } @else if (transfers.error()) {
        <app-error-state [message]="errorMessage()" [requestId]="errorRequestId()" />
      } @else if (!transfers.value()?.items?.length) {
        <app-empty
          message="No transfers recorded yet."
          hint="A move appears here as soon as it is filed." />
      } @else {
        <div class="scroll-x">
          <table>
            <thead>
              <tr>
                <th>Date</th>
                <th>Player</th>
                <th>From</th>
                <th>To</th>
                <th>Type</th>
                <th class="right">Fee</th>
              </tr>
            </thead>
            <tbody>
              @for (transfer of transfers.value()!.items; track transfer.id) {
                <tr>
                  <td class="tabular muted">{{ transfer.transfer_date | date: 'd MMM y' }}</td>
                  <td>
                    <a [routerLink]="['/players', transfer.player_id]">Player</a>
                  </td>
                  <td>{{ clubName(transfer.from_team_id) }}</td>
                  <td>{{ clubName(transfer.to_team_id) }}</td>
                  <td><span class="type">{{ label(transfer.transfer_type) }}</span></td>
                  <!--
                    A null fee is *undisclosed*, which is a different fact from
                    a free transfer. MoneyPipe renders it as a dash rather than
                    as zero, so the table never asserts a price that was never
                    stated.
                  -->
                  <td class="right tabular">
                    {{ transfer.fee_minor | money: { currency: transfer.currency, compact: true, emptyLabel: '—' } }}
                  </td>
                </tr>
              }
            </tbody>
          </table>
        </div>

        <app-paginator
          [page]="transfers.value()!"
          label="transfer results"
          (pageChange)="goTo($event)" />
      }
    </main>
  `,
  styles: `
    .feed { padding-block: var(--space-6); }
    header { margin-bottom: var(--space-5); }
    h1 { margin-bottom: var(--space-2); }
    .standfirst { color: var(--ink-soft); }

    .filters {
      display: flex;
      flex-wrap: wrap;
      gap: var(--space-2);
      margin-bottom: var(--space-5);
    }
    .filters button {
      background: var(--surface);
      border: 1px solid var(--line);
      border-radius: 999px;
      padding: var(--space-1) var(--space-3);
      font-size: var(--text-sm);
      color: var(--ink-soft);
      cursor: pointer;
    }
    .filters button.active {
      border-color: var(--accent);
      color: var(--accent);
      background: var(--accent-soft);
    }

    table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
    th {
      text-align: left;
      font-size: var(--text-xs);
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--muted);
      padding: var(--space-3);
      background: var(--surface-2);
      white-space: nowrap;
    }
    td { padding: var(--space-3); border-bottom: 1px solid var(--line-soft); }
    .right { text-align: right; }
    .muted { color: var(--muted); white-space: nowrap; }
    .type {
      font-family: var(--font-mono);
      font-size: 11px;
      color: var(--ink-soft);
    }
  `,
})
export class TransferFeed {
  private readonly reader = inject(TRANSFER_READER);
  private readonly lookup = inject(LookupStore);

  protected readonly types = TYPES;
  protected readonly type = signal<TransferType | ''>('');
  private readonly page = signal<PageQuery>({ limit: 25, offset: 0 });

  protected readonly transfers = resource({
    params: () => ({ type: this.type(), page: this.page() }),
    loader: async ({ params }) => {
      // Club names are needed to render a row, so both land before the table
      // does — otherwise every club flashes as a dash and then fills in.
      await this.lookup.loadTeams();
      return this.reader.list({
        ...params.page,
        type: params.type || undefined,
      });
    },
  });

  protected readonly errorMessage = computed(() => {
    const error = this.transfers.error();
    return error instanceof Error ? error.message : 'Could not load transfers.';
  });

  protected readonly errorRequestId = computed(() => {
    const error = this.transfers.error();
    return error instanceof ApiError ? (error.requestId ?? null) : null;
  });

  /** A null club is a real fact: arriving from nowhere, or leaving the game. */
  protected clubName(id: string | null): string {
    return this.lookup.teamName(id, '—');
  }

  protected label(type: TransferType): string {
    return TYPES.find((t) => t.value === type)?.label ?? type;
  }

  protected setType(type: TransferType | ''): void {
    this.type.set(type);
    this.page.set({ limit: 25, offset: 0 });
  }

  protected goTo(query: PageQuery): void {
    this.page.set(query);
  }
}

/** Re-exported so the route file does not need to know the Transfer type. */
export type { Transfer };
