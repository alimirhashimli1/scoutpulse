import { ChangeDetectionStrategy, Component, input } from '@angular/core';

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
  template: `<p class="state" role="status">{{ message() }}</p>`,
  styles: `
    .state {
      color: var(--muted);
      padding-block: var(--space-6);
      font-size: var(--text-sm);
    }
  `,
})
export class Loading {
  readonly message = input('Loading…');
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
    .headline { color: var(--ink-soft); margin-inline: auto; }
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
        <p class="reference">Reference: <code>{{ requestId() }}</code></p>
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
    .headline { color: var(--critical); }
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
