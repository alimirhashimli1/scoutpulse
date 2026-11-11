import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  afterRenderEffect,
  effect,
  input,
  signal,
  viewChild,
} from '@angular/core';

/**
 * Text that takes up a fixed number of lines until asked for more.
 *
 * A step's caption sits on top of the pitch, and a board's description sits
 * above it. On a phone a long one would cover the board or push it off the
 * screen, so it shows a couple of lines and a "More" button, and only if it
 * does not fit.
 */
@Component({
  selector: 'app-clamp-text',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { '[class.open]': 'open()' },
  template: `
    <p #paragraph [class.clamped]="!open()" [style.--lines]="lines()">{{ text() }}</p>
    @if (overflows() || open()) {
      <button type="button" [attr.aria-expanded]="open()" (click)="open.set(!open())">
        {{ open() ? 'Less' : 'More' }}
      </button>
    }
  `,
  styles: `
    :host {
      display: block;
    }
    p {
      margin: 0;
      max-width: none;
      overflow-wrap: anywhere;
    }
    .clamped {
      display: -webkit-box;
      -webkit-box-orient: vertical;
      -webkit-line-clamp: var(--lines);
      line-clamp: var(--lines);
      overflow: hidden;
    }
    button {
      border: 0;
      padding: 2px 0 0;
      background: none;
      color: inherit;
      font: inherit;
      font-size: var(--text-xs);
      font-weight: 700;
      text-decoration: underline;
      cursor: pointer;
      pointer-events: auto;
    }
  `,
})
export class ClampText {
  readonly text = input.required<string>();
  readonly lines = input(2);

  protected readonly open = signal(false);
  protected readonly overflows = signal(false);
  private readonly paragraph = viewChild.required<ElementRef<HTMLElement>>('paragraph');

  constructor() {
    // A new caption starts folded again.
    effect(() => {
      this.text();
      this.open.set(false);
    });
    afterRenderEffect((onCleanup) => {
      this.text();
      if (this.open()) return;
      const el = this.paragraph().nativeElement;
      const measure = () => this.overflows.set(el.scrollHeight > el.clientHeight + 1);
      measure();
      if (typeof ResizeObserver === 'undefined') return;
      const watcher = new ResizeObserver(measure);
      watcher.observe(el);
      onCleanup(() => watcher.disconnect());
    });
  }
}
