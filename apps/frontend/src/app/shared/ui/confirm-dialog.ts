import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  effect,
  input,
  model,
  output,
  viewChild,
} from '@angular/core';

/**
 * A confirmation dialog, replacing `window.confirm`.
 *
 * Built on the native `<dialog>` element rather than a hand-rolled overlay,
 * which is what makes it short: `showModal()` gives focus trapping, an inert
 * background, Escape-to-close and a backdrop pseudo-element without any of it
 * being reimplemented. A div-based modal has to do all four by hand and
 * usually gets at least one wrong.
 *
 * The destructive action is **not** the autofocused button. Someone who opens
 * this by accident and hits Enter should cancel, not delete.
 */
@Component({
  selector: 'app-confirm-dialog',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <dialog #dialog (close)="dismiss()" (click)="onBackdropClick($event)">
      <!--
        The click handler above fires for the backdrop too, because the
        backdrop is part of the dialog element. This inner wrapper stops
        clicks inside the panel from being read as clicks outside it.
      -->
      <div class="panel" (click)="$event.stopPropagation()">
        <h2>{{ title() }}</h2>
        @if (message()) {
          <p class="message">{{ message() }}</p>
        }

        <div class="actions">
          <button class="btn" type="button" (click)="cancel()" autofocus>
            {{ cancelLabel() }}
          </button>
          <button
            type="button"
            class="btn"
            [class.danger]="danger()"
            [class.primary]="!danger()"
            (click)="accept()"
          >
            {{ confirmLabel() }}
          </button>
        </div>
      </div>
    </dialog>
  `,
  styles: `
    dialog {
      padding: 0;
      border: none;
      background: transparent;
      max-width: min(28rem, calc(100vw - 2rem));
    }
    dialog::backdrop {
      /* Deliberately not a theme token: ::backdrop is outside the page's
         cascade, so custom properties defined on :root do not reach it. */
      background: rgb(10 12 14 / 55%);
    }
    .panel {
      background: var(--surface);
      color: var(--ink);
      border: 1px solid var(--line);
      border-radius: var(--radius-lg);
      box-shadow: var(--shadow);
      padding: var(--space-5);
    }
    h2 {
      font-size: var(--text-lg);
      margin: 0 0 var(--space-3);
    }
    .message {
      color: var(--ink-soft);
      font-size: var(--text-sm);
      margin: 0 0 var(--space-5);
    }
    .actions {
      display: flex;
      gap: var(--space-2);
      justify-content: flex-end;
      flex-wrap: wrap;
    }
  `,
})
export class ConfirmDialog {
  /** Two-way: set it to open, and it clears itself however the dialog closes. */
  readonly open = model(false);

  readonly title = input.required<string>();
  readonly message = input('');
  readonly confirmLabel = input('Confirm');
  readonly cancelLabel = input('Cancel');
  /** Styles the confirm button as destructive. */
  readonly danger = input(false);

  /** Emitted only when the confirm button is pressed. */
  readonly confirmed = output<void>();

  private readonly dialog = viewChild<ElementRef<HTMLDialogElement>>('dialog');

  constructor() {
    effect(() => {
      const element = this.dialog()?.nativeElement;
      if (!element) return;

      // showModal throws if called on an already-open dialog, and close() on a
      // closed one is a no-op that still fires nothing — so both are guarded
      // by the element's own state rather than by tracking it separately.
      if (this.open() && !element.open) {
        element.showModal();
      } else if (!this.open() && element.open) {
        element.close();
      }
    });
  }

  protected accept(): void {
    this.open.set(false);
    this.confirmed.emit();
  }

  protected cancel(): void {
    this.open.set(false);
  }

  /**
   * Closing by Escape or any other route the browser owns.
   *
   * The model has to be cleared here as well, or a dialog dismissed with
   * Escape would leave `open` true and refuse to reopen.
   */
  protected dismiss(): void {
    this.open.set(false);
  }

  protected onBackdropClick(event: MouseEvent): void {
    // Reached only when the click was not stopped by the panel, i.e. it landed
    // on the backdrop. Dismissing there is the convention, and it is safe
    // because dismissing never confirms.
    if (event.target === this.dialog()?.nativeElement) {
      this.cancel();
    }
  }
}
