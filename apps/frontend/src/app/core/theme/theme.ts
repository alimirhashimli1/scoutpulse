import { DOCUMENT, isPlatformBrowser } from '@angular/common';
import { Injectable, PLATFORM_ID, computed, inject, signal } from '@angular/core';

export type ThemeChoice = 'system' | 'light' | 'dark';

export const THEME_STORAGE_KEY = 'scoutpulse.theme';

/**
 * Light, dark, or whatever the operating system says.
 *
 * Three states, not two. A boolean "dark mode" cannot express *"follow my
 * system"*, so a visitor who once tapped the toggle would be pinned to that
 * choice forever, including when their OS switches at sunset. `system` is the
 * default and is reachable again by cycling.
 *
 * The mechanism is entirely in CSS. `styles.css` defines the palette three
 * times: on bare `:root`, under `prefers-color-scheme: dark` guarded against
 * an explicit light choice, and again under `:root[data-theme='dark']` so the
 * toggle wins in both directions. All this class does is stamp the attribute.
 *
 * **The first paint is handled in index.html, not here.** A small inline
 * script applies the stored choice before the stylesheet is evaluated. Doing
 * it from Angular would be too late: the page would paint in the system theme
 * and then swap, which is the flash this exists to avoid — and it is most
 * visible in exactly the case someone set the preference for.
 */
@Injectable({ providedIn: 'root' })
export class ThemeStore {
  private readonly document = inject(DOCUMENT);
  private readonly isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  private readonly _choice = signal<ThemeChoice>(this.read());

  readonly choice = this._choice.asReadonly();

  /** What to call the *next* state, for a button that cycles through three. */
  readonly nextLabel = computed<string>(() => {
    switch (this._choice()) {
      case 'system':
        return 'Switch to light';
      case 'light':
        return 'Switch to dark';
      default:
        return 'Use system theme';
    }
  });

  readonly icon = computed<string>(() => {
    switch (this._choice()) {
      case 'light':
        return '☀';
      case 'dark':
        return '☾';
      default:
        return '◐';
    }
  });

  set(choice: ThemeChoice): void {
    this._choice.set(choice);
    if (!this.isBrowser) return;

    const root = this.document.documentElement;
    if (choice === 'system') {
      // Removed, not set to "system". The CSS keys off the attribute being
      // absent, which is what lets prefers-color-scheme take over again.
      root.removeAttribute('data-theme');
    } else {
      root.setAttribute('data-theme', choice);
    }

    try {
      this.document.defaultView?.localStorage.setItem(THEME_STORAGE_KEY, choice);
    } catch {
      // Private browsing, or storage disabled. The choice still applies for
      // this page; it simply will not survive a reload. Not worth failing over.
    }
  }

  cycle(): void {
    const order: ThemeChoice[] = ['system', 'light', 'dark'];
    const next = order[(order.indexOf(this._choice()) + 1) % order.length];
    this.set(next);
  }

  private read(): ThemeChoice {
    // The server has no storage and no visitor preference — it renders the
    // system default, and the inline script corrects it before paint.
    if (!this.isBrowser) return 'system';

    try {
      const stored = this.document.defaultView?.localStorage.getItem(THEME_STORAGE_KEY);
      return stored === 'light' || stored === 'dark' ? stored : 'system';
    } catch {
      return 'system';
    }
  }
}
