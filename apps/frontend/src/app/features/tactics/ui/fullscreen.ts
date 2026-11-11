import { DestroyRef, Signal, inject, signal } from '@angular/core';

/**
 * Whether a CSS media query matches, kept current. False on the server and
 * anywhere else without `matchMedia`.
 */
export function mediaQuery(query: string): Signal<boolean> {
  const list = globalThis.matchMedia?.(query);
  const matches = signal(list?.matches ?? false);
  if (list) {
    const update = (e: MediaQueryListEvent) => matches.set(e.matches);
    list.addEventListener('change', update);
    inject(DestroyRef).onDestroy(() => list.removeEventListener('change', update));
  }
  return matches.asReadonly();
}

/**
 * Full screen for a board.
 *
 * The page's own CSS does the work: `active` makes the target cover the
 * viewport. The browser's Fullscreen API is asked on top of that, to hide its
 * own bars, but is not relied on, because iPhones refuse it for anything but
 * a video and full screen has to work there too.
 */
export class BoardFullscreen {
  readonly active = signal(false);
  private native = false;

  constructor(
    private readonly document: Document,
    private readonly target: () => HTMLElement | undefined,
    /** Runs just before the board changes shape. */
    private readonly beforeChange: () => void = () => {},
  ) {}

  toggle(): void {
    if (this.active()) this.exit();
    else this.enter();
  }

  enter(): void {
    const el = this.target();
    if (!el || this.active()) return;
    this.beforeChange();
    this.active.set(true);
    this.document.documentElement.style.overflow = 'hidden';
    void this.requestNative(el);
  }

  exit(): void {
    if (!this.active()) return;
    if (this.native && this.document.fullscreenElement) void this.document.exitFullscreen();
    this.leave();
  }

  /** The browser left full screen on its own: Esc, or the back gesture. */
  sync(): void {
    if (this.native && !this.document.fullscreenElement) this.leave();
  }

  /** The page is going away, so nothing of full screen may outlive it. */
  release(): void {
    this.exit();
  }

  private async requestNative(el: HTMLElement): Promise<void> {
    try {
      await el.requestFullscreen?.();
      this.native = !!this.document.fullscreenElement;
      // A pitch is wider than it is tall. Phones that allow it turn sideways.
      if (this.native && globalThis.matchMedia?.('(pointer: coarse)').matches)
        await lockable()?.lock?.('landscape');
    } catch {
      // Refused or unsupported: the page's own full screen is already showing.
    }
  }

  private leave(): void {
    this.beforeChange();
    this.native = false;
    this.active.set(false);
    this.document.documentElement.style.overflow = '';
    try {
      lockable()?.unlock?.();
    } catch {
      // Nothing was locked.
    }
  }
}

/** `lock` is missing from the DOM typings and from some browsers. */
function lockable() {
  return globalThis.screen?.orientation as
    | { lock?(o: string): Promise<void>; unlock?(): void }
    | undefined;
}
