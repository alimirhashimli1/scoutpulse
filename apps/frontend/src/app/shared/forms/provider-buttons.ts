import { ChangeDetectionStrategy, Component, inject, input, resource } from '@angular/core';
import { TitleCasePipe } from '@angular/common';

import { AuthRepository } from '../../core/auth/auth-repository';

/**
 * "Continue with Google" and friends.
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
 * Google button that 404s because no credentials are set is worse than
 * rendering none.
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
            Continue with {{ provider | titlecase }}
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
      display: block;
      text-align: center;
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
  `,
})
export class ProviderButtons {
  private readonly auth = inject(AuthRepository);

  readonly label = input('or');

  protected readonly providers = resource({
    loader: () => this.auth.providers(),
  });

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
