import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';

import { Page, PageQuery } from '../../core/api/page';

/**
 * Paging control, driven by the envelope every list endpoint returns.
 *
 * It takes the `Page<T>` as it arrived rather than separate page and total
 * inputs, so no caller has to derive anything — and there is no second place
 * for the offset arithmetic to be got wrong.
 *
 * There is no "page 4 of 27": the API reports `has_more`, not a total, because
 * counting every matching row on every request is expensive and nothing in the
 * product needs the number. Next and previous is what the data supports, so it
 * is what this offers.
 */
@Component({
  selector: 'app-paginator',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (hasAnyPaging()) {
      <nav class="paginator" [attr.aria-label]="label()">
        <button
          type="button"
          [disabled]="!hasPrevious()"
          (click)="goPrevious()"
        >
          ← Previous
        </button>

        <span class="range tabular" aria-live="polite">
          {{ rangeLabel() }}
        </span>

        <button
          type="button"
          [disabled]="!page().has_more"
          (click)="goNext()"
        >
          Next →
        </button>
      </nav>
    }
  `,
  styles: `
    .paginator {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: var(--space-4);
      padding-block: var(--space-5);
      border-top: 1px solid var(--line);
      margin-top: var(--space-4);
    }
    button {
      background: var(--surface);
      color: var(--ink);
      border: 1px solid var(--line);
      border-radius: var(--radius);
      padding: var(--space-2) var(--space-4);
      cursor: pointer;
    }
    button:hover:not(:disabled) {
      border-color: var(--accent);
      color: var(--accent);
    }
    button:disabled {
      opacity: 0.4;
      cursor: default;
    }
    .range {
      font-size: var(--text-sm);
      color: var(--muted);
    }
  `,
})
export class Paginator<T> {
  readonly page = input.required<Page<T>>();
  /** Names the control for assistive technology: "player results", say. */
  readonly label = input('pagination');

  readonly pageChange = output<PageQuery>();

  protected readonly hasPrevious = computed(() => this.page().offset > 0);

  /** Hidden entirely when everything fits on one page. */
  protected readonly hasAnyPaging = computed(
    () => this.page().has_more || this.page().offset > 0,
  );

  protected readonly rangeLabel = computed(() => {
    const { offset, items } = this.page();
    if (items.length === 0) return 'No results';
    return `${offset + 1}–${offset + items.length}`;
  });

  protected goPrevious(): void {
    const { limit, offset } = this.page();
    this.pageChange.emit({ limit, offset: Math.max(0, offset - limit) });
  }

  protected goNext(): void {
    const { limit, offset } = this.page();
    this.pageChange.emit({ limit, offset: offset + limit });
  }
}
