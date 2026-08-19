import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  afterRenderEffect,
  inject,
  input,
} from '@angular/core';

/**
 * A labelled form control: label, optional hint, optional error.
 *
 * Content-projected rather than wrapping an input, because the controls differ
 * — text, select, date, number — and a component that renders all of them
 * behind a `type` input is a switch statement pretending to be an abstraction.
 *
 * What it does own is the accessibility wiring, which is the part every form
 * otherwise forgets: the label points at the control, and the hint and error
 * are announced with it rather than sitting nearby as decoration.
 */
@Component({
  selector: 'app-field',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="field" [class.invalid]="!!error()">
      <label [attr.for]="for()">
        {{ label() }}
        @if (optional()) {
          <span class="optional">optional</span>
        }
      </label>

      <ng-content />

      @if (hint()) {
        <p class="hint" [id]="for() + '-hint'">{{ hint() }}</p>
      }
      @if (error()) {
        <p class="error" [id]="for() + '-error'" role="alert">{{ error() }}</p>
      }
    </div>
  `,
  styles: `
    .field {
      display: flex;
      flex-direction: column;
      gap: var(--space-2);
    }
    label {
      font-size: var(--text-xs);
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--muted);
      display: flex;
      gap: var(--space-2);
      align-items: baseline;
    }
    .optional {
      font-weight: 400;
      letter-spacing: 0;
      text-transform: none;
      font-size: var(--text-xs);
      color: var(--muted);
      opacity: 0.8;
    }
    .hint {
      font-size: var(--text-xs);
      color: var(--muted);
      margin: 0;
    }
    .error {
      font-size: var(--text-xs);
      color: var(--critical);
      margin: 0;
    }

    /*
      The control itself is styled in styles.css, not here. It arrives by
      content projection, and Angular's emulated encapsulation stamps projected
      content with the parent component's attribute — so a rule written in this
      file would compile to a selector the input can never match.
    */
  `,
})
export class Field {
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  /** The id of the control being labelled. Required — a label with no target is decoration. */
  readonly for = input.required<string>();
  readonly label = input.required<string>();
  readonly hint = input<string | null>(null);
  readonly error = input<string | null>(null);
  /**
   * Marks the field optional rather than marking the others required.
   *
   * Most fields on these forms are optional — the API asks for very little —
   * so flagging the minority is less noise than an asterisk on everything.
   */
  readonly optional = input(false);

  constructor() {
    /*
      Points the control at its hint and error.

      This component's whole claim is that it owns the accessibility wiring,
      and until now it rendered `<p id="name-hint">` that nothing referenced —
      so a screen reader announced the label and then fell silent about the
      constraint the field was rejecting on.

      The control is projected, so the attribute cannot be bound in the
      template; it is set on the element after render instead. `afterRenderEffect`
      re-runs when the error appears or clears, and does not run during server
      rendering — which is correct, because these attributes only mean anything
      to assistive technology reading a live document.
    */
    afterRenderEffect(() => {
      const control = this.host.nativeElement.querySelector<HTMLElement>('input, select, textarea');
      if (!control) return;

      const described = [
        this.hint() ? `${this.for()}-hint` : null,
        this.error() ? `${this.for()}-error` : null,
      ].filter((id): id is string => id !== null);

      if (described.length > 0) {
        control.setAttribute('aria-describedby', described.join(' '));
      } else {
        control.removeAttribute('aria-describedby');
      }

      // Announced by the control itself, so someone tabbing back to a field
      // they already failed hears that it is still invalid.
      control.setAttribute('aria-invalid', this.error() ? 'true' : 'false');
    });
  }
}
