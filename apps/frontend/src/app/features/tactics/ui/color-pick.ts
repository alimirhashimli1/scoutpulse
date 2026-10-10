import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';

import { PALETTE } from '../board/model';

/** A row of swatches plus "any colour", optionally with a "none" choice. */
@Component({
  selector: 'app-color-pick',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="row" role="radiogroup" [attr.aria-label]="label()">
      @if (allowNone()) {
        <button
          type="button"
          class="sw none"
          role="radio"
          [attr.aria-checked]="!value()"
          [attr.aria-label]="noneLabel()"
          [title]="noneLabel()"
          (click)="picked.emit(null)"
        ></button>
      }
      @for (c of swatches(); track c) {
        <button
          type="button"
          class="sw"
          role="radio"
          [attr.aria-checked]="value() === c"
          [attr.aria-label]="c"
          [style.background]="c"
          (click)="picked.emit(c)"
        ></button>
      }
      <label class="sw any" title="Any colour">
        <span class="visually-hidden">Any colour</span>
        <input
          type="color"
          [value]="value() || '#ffffff'"
          (input)="picked.emit($any($event.target).value)"
        />
      </label>
    </div>
  `,
  styles: `
    .row {
      display: flex;
      flex-wrap: wrap;
      gap: 5px;
    }
    .sw {
      width: 1.35rem;
      height: 1.35rem;
      border-radius: 50%;
      border: 1px solid var(--line);
      padding: 0;
      cursor: pointer;
      position: relative;
    }
    .sw[aria-checked='true'] {
      outline: 2px solid var(--accent);
      outline-offset: 2px;
    }
    .none {
      background:
        linear-gradient(135deg, transparent 45%, var(--critical) 45% 55%, transparent 55%),
        var(--surface);
    }
    .any {
      background: conic-gradient(#f43f5e, #facc15, #22c55e, #22d3ee, #3b82f6, #a855f7, #f43f5e);
      overflow: hidden;
    }
    .any input {
      position: absolute;
      inset: 0;
      opacity: 0;
      cursor: pointer;
    }
  `,
})
export class ColorPick {
  readonly value = input<string | null | undefined>(null);
  readonly label = input('Colour');
  readonly allowNone = input(false);
  readonly noneLabel = input('None');
  readonly swatches = input<readonly string[]>(PALETTE.slice(0, 12));
  readonly picked = output<string | null>();
}
