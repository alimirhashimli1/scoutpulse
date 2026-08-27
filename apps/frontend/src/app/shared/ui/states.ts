import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

/**
 * The three states every list and detail view has besides "loaded".
 *
 * Kept together because they are always needed together, and because a list
 * that renders nothing while loading and nothing when empty is indistinguishable
 * from a broken one.
 */

@Component({
  selector: 'app-loading',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <!--
      The message is for screen readers, and the bars are for everyone else.

      role="status" with aria-live announces the wait without stealing focus;
      the skeleton is aria-hidden because "loading, loading, loading" read out
      once per bar is noise. A plain text line was the previous behaviour and
      made a loading list look like an empty one.
    -->
    <div class="loading">
      <p class="visually-hidden" role="status" aria-live="polite">{{ message() }}</p>
      <div class="skeleton" aria-hidden="true">
        @for (bar of bars(); track $index) {
          <div class="bar" [style.width.%]="bar"></div>
        }
      </div>
    </div>
  `,
  styles: `
    .loading {
      padding-block: var(--space-5);
    }
    .skeleton {
      display: flex;
      flex-direction: column;
      gap: var(--space-3);
    }
    .bar {
      height: 1rem;
      border-radius: var(--radius-sm);
      background: linear-gradient(
        90deg,
        var(--surface-2) 25%,
        var(--line-soft) 37%,
        var(--surface-2) 63%
      );
      background-size: 400% 100%;
      animation: shimmer 1.4s ease-in-out infinite;
    }

    @keyframes shimmer {
      from {
        background-position: 100% 0;
      }
      to {
        background-position: 0 0;
      }
    }

    /*
      The global reduced-motion rule cuts the duration to near zero, which
      would leave the gradient frozen mid-sweep at whatever position it
      happened to stop. A flat fill is the honest still frame.
    */
    @media (prefers-reduced-motion: reduce) {
      .bar {
        animation: none;
        background: var(--surface-2);
      }
    }
  `,
})
export class Loading {
  readonly message = input('Loading…');

  /** How many placeholder rows, and how wide. Uneven widths read as text. */
  readonly lines = input(3);

  protected readonly bars = computed(() => {
    const widths = [100, 82, 91, 68, 95, 76];
    return Array.from({ length: this.lines() }, (_, i) => widths[i % widths.length]);
  });
}

@Component({
  selector: 'app-empty',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="state">
      <p class="headline">{{ message() }}</p>
      @if (hint()) {
        <p class="hint">{{ hint() }}</p>
      }
    </div>
  `,
  styles: `
    .state {
      padding: var(--space-7) var(--space-4);
      text-align: center;
      border: 1px dashed var(--line);
      border-radius: var(--radius);
      background: var(--surface);
    }
    .headline {
      color: var(--ink-soft);
      margin-inline: auto;
    }
    .hint {
      color: var(--muted);
      font-size: var(--text-sm);
      margin: var(--space-2) auto 0;
    }
  `,
})
export class Empty {
  readonly message = input('Nothing here yet.');
  /** What the reader could do about it, when there is something. */
  readonly hint = input<string | null>(null);
}

@Component({
  selector: 'app-error-state',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="state" role="alert">
      <p class="headline">{{ message() }}</p>
      @if (requestId()) {
        <!--
          The same id appears in the service logs. Showing it means a user can
          quote something findable instead of describing what they were doing.
        -->
        <p class="reference">
          Reference: <code>{{ requestId() }}</code>
        </p>
      }
    </div>
  `,
  styles: `
    .state {
      padding: var(--space-5);
      border: 1px solid var(--critical);
      background: var(--critical-soft);
      border-radius: var(--radius);
    }
    .headline {
      color: var(--critical);
    }
    .reference {
      margin-top: var(--space-2);
      font-size: var(--text-xs);
      color: var(--ink-soft);
    }
  `,
})
export class ErrorState {
  readonly message = input('Something went wrong.');
  readonly requestId = input<string | null>(null);
}
