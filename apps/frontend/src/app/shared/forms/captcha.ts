import { DOCUMENT, isPlatformBrowser } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  PLATFORM_ID,
  afterNextRender,
  inject,
  input,
  model,
  viewChild,
} from '@angular/core';

/**
 * The human-challenge widget.
 *
 * Renders whichever provider the server says is configured, and emits the
 * token it produces. **It proves nothing on its own** — a client that wants to
 * skip the challenge simply does not render this and posts to the API
 * directly. What stops automated sign-ups is identity-svc refusing a request
 * whose token the provider will not vouch for; this is the part that lets a
 * genuine visitor produce one.
 *
 * Nothing renders when no provider is configured, matching how the sign-in
 * buttons behave: a widget that cannot work is worse than its absence.
 */
@Component({
  selector: 'app-captcha',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (siteKey()) {
      <div class="captcha">
        <div #host></div>
      </div>
    }
  `,
  styles: `
    .captcha {
      margin-top: var(--space-4);
      /* The widget is a fixed-size iframe; letting it overflow a narrow form
         is the usual way this breaks on a phone. */
      max-width: 100%;
      overflow-x: auto;
    }
  `,
})
export class Captcha {
  private readonly document = inject(DOCUMENT);
  private readonly isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  /** "turnstile" or "recaptcha". Anything else renders nothing. */
  readonly provider = input<string>('');
  readonly siteKey = input<string>('');

  /** The token to send with the form. Empty until the visitor passes. */
  readonly token = model<string>('');

  private readonly host = viewChild<ElementRef<HTMLElement>>('host');

  constructor() {
    // afterNextRender rather than an effect: the script has to attach to a
    // real element, and it must not run during server rendering, where there
    // is no DOM and no visitor to challenge.
    afterNextRender(() => void this.render());
  }

  private async render(): Promise<void> {
    const host = this.host()?.nativeElement;
    if (!this.isBrowser || !host || !this.siteKey()) return;

    const config = SCRIPTS[this.provider()];
    if (!config) return;

    try {
      await this.loadScript(config.src, config.global);
    } catch {
      // A blocked or unreachable widget script leaves the token empty, and the
      // server rejects the submission with a message about the challenge.
      // Failing visibly there beats a silent no-op here.
      return;
    }

    const api = (globalThis as Record<string, unknown>)[config.global] as
      { render: (el: HTMLElement, opts: Record<string, unknown>) => unknown } | undefined;

    api?.render(host, {
      sitekey: this.siteKey(),
      callback: (token: string) => this.token.set(token),
      // Both providers issue tokens that expire, and a stale one is rejected
      // server-side. Clearing it means the form reports "complete the
      // challenge" rather than "challenge failed".
      'expired-callback': () => this.token.set(''),
      'error-callback': () => this.token.set(''),
    });
  }

  /**
   * Loads a provider script once per page.
   *
   * Both widgets install a global and are not safe to inject twice, so an
   * existing tag is awaited rather than a second one added — which is what
   * happens when a visitor navigates between sign-in and sign-up.
   */
  private loadScript(src: string, global: string): Promise<void> {
    if ((globalThis as Record<string, unknown>)[global]) return Promise.resolve();

    const existing = this.document.querySelector<HTMLScriptElement>(`script[src="${src}"]`);
    if (existing) {
      return new Promise((resolve, reject) => {
        existing.addEventListener('load', () => resolve(), { once: true });
        existing.addEventListener('error', () => reject(new Error('captcha script failed')), {
          once: true,
        });
      });
    }

    return new Promise((resolve, reject) => {
      const script = this.document.createElement('script');
      script.src = src;
      script.async = true;
      script.defer = true;
      script.addEventListener('load', () => resolve(), { once: true });
      script.addEventListener('error', () => reject(new Error('captcha script failed')), {
        once: true,
      });
      this.document.head.appendChild(script);
    });
  }
}

/**
 * The two providers, which share an API shape close enough for one component.
 *
 * `render=explicit` matters: without it both scripts auto-render into any
 * element carrying their class the moment they load, which in a
 * single-page app means rendering into a page that has since been replaced.
 */
const SCRIPTS: Record<string, { src: string; global: string }> = {
  turnstile: {
    src: 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit',
    global: 'turnstile',
  },
  recaptcha: {
    src: 'https://www.google.com/recaptcha/api.js?render=explicit',
    global: 'grecaptcha',
  },
};
