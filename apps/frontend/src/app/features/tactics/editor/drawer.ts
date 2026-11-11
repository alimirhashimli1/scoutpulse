import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  computed,
  inject,
  input,
  model,
  signal,
} from '@angular/core';

/** How far a panel must be pulled, as a share of its width, to change state. */
const SNAP = 0.25;

/**
 * A panel that slides over the board from one side, pulled in and out by a
 * tab on its edge (a tap on the tab works too).
 *
 * When not `enabled` it is invisible to the layout: its content stands
 * directly in the page's grid, as if the drawer were not there.
 */
@Component({
  selector: 'app-drawer',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    '[class.on]': 'enabled()',
    '[class.open]': 'open()',
    '[class.dragging]': 'pull() !== null',
    '[class]': 'side()',
    '[style.--pull.px]': 'pull() ?? 0',
  },
  template: `
    <div class="panel" [attr.inert]="enabled() && !open() ? '' : null"><ng-content /></div>
    @if (enabled()) {
      <button
        type="button"
        class="handle"
        [class.attention]="attention()"
        [attr.aria-expanded]="open()"
        [attr.aria-label]="handleLabel()"
        [title]="handleLabel()"
        (pointerdown)="grab($event)"
        (pointermove)="drag($event)"
        (pointerup)="release()"
        (pointercancel)="release()"
        (click)="tapped()"
      >
        <span class="grip" aria-hidden="true"></span>
        <span class="word">{{ label() }}</span>
      </button>
    }
  `,
  styles: `
    :host,
    .panel {
      display: contents;
    }
    :host(.on) {
      --size: min(12rem, 70vw);
      display: block;
      position: absolute;
      top: 0;
      bottom: 0;
      z-index: 6;
      width: var(--size);
      transition: transform 0.22s ease;
    }
    :host(.on.right) {
      --size: min(17rem, 82vw);
      right: 0;
      transform: translateX(calc(100% + var(--pull)));
    }
    :host(.on.left) {
      left: 0;
      transform: translateX(calc(-100% + var(--pull)));
    }
    :host(.on.open) {
      transform: translateX(var(--pull));
    }
    :host(.on.dragging) {
      transition: none;
    }
    :host(.on) .panel {
      display: flex;
      flex-direction: column;
      height: 100%;
      box-shadow: 0 0 24px rgb(0 0 0 / 30%);
    }
    .handle {
      position: absolute;
      top: 34%;
      width: 2rem;
      height: 6.5rem;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      gap: 6px;
      padding: 0;
      border: 0;
      background: var(--accent);
      color: var(--accent-ink);
      font-size: var(--text-xs);
      font-weight: 700;
      cursor: grab;
      touch-action: none;
      box-shadow: 0 2px 10px rgb(0 0 0 / 35%);
    }
    :host(.left) .handle {
      left: 100%;
      border-radius: 0 var(--radius-lg) var(--radius-lg) 0;
    }
    :host(.right) .handle {
      right: 100%;
      border-radius: var(--radius-lg) 0 0 var(--radius-lg);
    }
    .handle.attention {
      background: var(--warning);
    }
    .grip {
      width: 0.5rem;
      height: 1rem;
      border-inline: 2px solid currentColor;
      opacity: 0.7;
    }
    .word {
      writing-mode: vertical-rl;
    }
  `,
})
export class Drawer {
  readonly side = input.required<'left' | 'right'>();
  readonly label = input.required<string>();
  /** Off, the content simply stands in the page's layout. */
  readonly enabled = input(false);
  /** Picks out the tab when there is something new in the panel. */
  readonly attention = input(false);
  readonly open = model(false);

  /** How far the panel is being pulled, in pixels; null when it is not. */
  protected readonly pull = signal<number | null>(null);
  protected readonly handleLabel = computed(
    () => `${this.open() ? 'Hide' : 'Show'} ${this.label().toLowerCase()}. Drag or tap.`,
  );

  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private start: number | null = null;
  private dragged = false;

  protected grab(e: PointerEvent): void {
    (e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId);
    this.start = e.clientX;
    this.dragged = false;
  }

  protected drag(e: PointerEvent): void {
    if (this.start === null) return;
    const dx = e.clientX - this.start;
    if (!this.dragged && Math.abs(dx) < 4) return;
    this.dragged = true;
    const size = this.host.nativeElement.offsetWidth;
    // Only as far as the panel can travel from where it stands.
    const towardsOpen = this.side() === 'left' ? 1 : -1;
    const [lo, hi] = this.open() ? [-size, 0] : [0, size];
    this.pull.set(Math.max(lo, Math.min(hi, dx * towardsOpen)) * towardsOpen);
  }

  protected release(): void {
    const pulled = this.pull();
    this.start = null;
    this.pull.set(null);
    if (pulled === null) return;
    const size = this.host.nativeElement.offsetWidth || 1;
    const out = (this.side() === 'left' ? pulled : -pulled) / size;
    if (this.open() ? out < -SNAP : out > SNAP) this.open.set(!this.open());
  }

  protected tapped(): void {
    // The click that ends a drag is not a tap.
    if (this.dragged) this.dragged = false;
    else this.open.set(!this.open());
  }
}
