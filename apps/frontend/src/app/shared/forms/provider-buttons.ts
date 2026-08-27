import { ChangeDetectionStrategy, Component, inject, input, resource } from '@angular/core';
import { TitleCasePipe } from '@angular/common';

import { AuthRepository } from '../../core/auth/auth-repository';

/**
 * "Continue with Gmail" and friends.
 *
 * Shared between sign-in and sign-up because with an external provider they
 * are the *same action*: the provider authenticates, and the account is
 * created on first arrival or matched on later ones. Registration previously
 * offered no provider at all, so someone who wanted to sign up with Google had
 * to know to go to the sign-in page instead — the button was not missing
 * because of configuration, it was never there.
 *
 * "Continue with" rather than "Sign in with" or "Sign up with" for the same
 * reason: one label is honest on both pages.
 *
 * Only providers this deployment has configured are offered. Rendering a
 * button that 404s because no credentials are set is worse than rendering
 * none.
 */
@Component({
  selector: 'app-provider-buttons',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [TitleCasePipe],
  template: `
    @if (providers.value()?.length) {
      <div class="providers">
        <span class="divider">{{ label() }}</span>
        @for (provider of providers.value()!; track provider) {
          <a class="provider" [href]="startUrl(provider)">
            @if (provider === 'google') {
              <!--
                Google's own mark, inline rather than linked: an external image
                is a request that can fail or be blocked, and a sign-in button
                that renders without its logo looks broken in a way that costs
                trust at exactly the wrong moment.

                aria-hidden because the label beside it already names the
                provider — a screen reader announcing "image, Google" before
                "Continue with Gmail" is noise, not information.
              -->
              <svg class="mark" viewBox="0 0 48 48" aria-hidden="true" focusable="false">
                <path
                  fill="#EA4335"
                  d="M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.72 17.74 9.5 24 9.5z"
                />
                <path
                  fill="#4285F4"
                  d="M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z"
                />
                <path
                  fill="#FBBC05"
                  d="M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z"
                />
                <path
                  fill="#34A853"
                  d="M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.15 1.45-4.92 2.3-8.16 2.3-6.26 0-11.57-4.22-13.47-9.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z"
                />
              </svg>
            }
            <span>Continue with {{ labelFor(provider) }}</span>
          </a>
        }
      </div>
    }
  `,
  styles: `
    .providers {
      margin-top: var(--space-6);
      display: grid;
      gap: var(--space-3);
    }
    .divider {
      text-align: center;
      color: var(--muted);
      font-size: var(--text-sm);
    }
    .provider {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: var(--space-3);
      padding: var(--space-3);
      border: 1px solid var(--line);
      border-radius: var(--radius);
      text-decoration: none;
      color: var(--ink);
    }
    .provider:hover {
      border-color: var(--accent);
      color: var(--accent);
    }
    /* Sized in em so the mark tracks the button's text rather than being
       pinned to a pixel size that stops matching when the type scale moves. */
    .mark {
      width: 1.15em;
      height: 1.15em;
      flex: none;
    }
  `,
})
export class ProviderButtons {
  private readonly auth = inject(AuthRepository);

  readonly label = input('or');

  protected readonly providers = resource({
    loader: () => this.auth.providers(),
  });

  /**
   * What to call each provider on the button.
   *
   * "Gmail" rather than "Google" is a deliberate choice: it is the name people
   * recognise for the account they are about to use. Anything not listed falls
   * back to its own name, so a provider added on the server appears with a
   * sensible label without needing a change here first.
   */
  private static readonly labels: Record<string, string> = {
    google: 'Gmail',
  };

  protected labelFor(provider: string): string {
    return ProviderButtons.labels[provider] ?? new TitleCasePipe().transform(provider);
  }

  /**
   * A full-page navigation, deliberately — not an XHR.
   *
   * The provider responds with a redirect to its consent screen, which the
   * user has to see and interact with. A fetch cannot follow that.
   */
  protected startUrl(provider: string): string {
    return this.auth.providerStartUrl(provider);
  }
}
