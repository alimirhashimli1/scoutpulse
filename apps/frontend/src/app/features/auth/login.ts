import { ChangeDetectionStrategy, Component, inject, resource, signal } from '@angular/core';
import { TitleCasePipe } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';

import { ApiError } from '../../core/api/api-error';
import { AuthFacade } from '../../core/auth/auth-facade';
import { AuthRepository } from '../../core/auth/auth-repository';
import { Captcha } from '../../shared/forms/captcha';

@Component({
  selector: 'app-login',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, TitleCasePipe, Captcha],
  template: `
    <main class="page auth">
      <h1>Sign in</h1>

      <form (ngSubmit)="submit()">
        <label for="identifier">Username or email</label>
        <input
          id="identifier"
          name="identifier"
          autocomplete="username"
          required
          [(ngModel)]="identifier"
        />

        <label for="password">Password</label>
        <input
          id="password"
          name="password"
          type="password"
          autocomplete="current-password"
          required
          [(ngModel)]="password"
        />

        @if (error()) {
          <p class="error" role="alert">{{ error() }}</p>
        }

        <app-captcha
          [provider]="config.value()?.captcha?.provider ?? ''"
          [siteKey]="config.value()?.captcha?.site_key ?? ''"
          [(token)]="captchaToken"
        />

        <button type="submit" [disabled]="busy()">
          {{ busy() ? 'Signing in…' : 'Sign in' }}
        </button>
      </form>

      <!--
        Only providers this deployment has configured are offered. Rendering a
        Google button that 404s because no credentials are set is worse than
        rendering none.
      -->
      @if (providers.value()?.length) {
        <div class="providers">
          <span class="divider">or</span>
          @for (provider of providers.value()!; track provider) {
            <a class="provider" [href]="startUrl(provider)">
              Continue with {{ provider | titlecase }}
            </a>
          }
        </div>
      }

      <p class="alt">No account? <a routerLink="/register">Create one</a></p>
    </main>
  `,
  styles: `
    .auth {
      max-width: 26rem;
      padding-block: var(--space-8);
    }
    h1 {
      margin-bottom: var(--space-6);
    }
    form {
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
      margin-top: var(--space-3);
    }
    input {
      padding: var(--space-3);
      border: 1px solid var(--line);
      border-radius: var(--radius);
      background: var(--surface);
    }
    button {
      margin-top: var(--space-5);
      padding: var(--space-3);
      background: var(--accent);
      color: var(--accent-ink);
      border: none;
      border-radius: var(--radius);
      cursor: pointer;
      font-weight: 600;
    }
    button:disabled {
      opacity: 0.6;
      cursor: default;
    }
    .error {
      color: var(--critical);
      background: var(--critical-soft);
      padding: var(--space-3);
      border-radius: var(--radius);
      margin-top: var(--space-4);
      font-size: var(--text-sm);
    }
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
    .alt {
      margin-top: var(--space-6);
      font-size: var(--text-sm);
      color: var(--muted);
    }
  `,
})
export class Login {
  private readonly facade = inject(AuthFacade);
  private readonly auth = inject(AuthRepository);
  private readonly router = inject(Router);

  protected readonly identifier = signal('');
  protected readonly password = signal('');
  protected readonly captchaToken = signal('');
  protected readonly busy = signal(false);
  protected readonly error = signal<string | null>(null);
  /** Set when the API refuses because the address is unconfirmed. */
  protected readonly needsVerification = signal(false);

  protected readonly providers = resource({
    loader: () => this.auth.providers(),
  });

  /**
   * Whether a challenge is required, and which widget renders it.
   *
   * Asked of the server rather than decided here: a client that simply did not
   * render the widget would otherwise opt itself out of the check.
   */
  protected readonly config = resource({
    loader: () => this.auth.authConfig(),
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

  protected async submit(): Promise<void> {
    if (this.busy()) return;

    this.busy.set(true);
    this.error.set(null);

    try {
      await this.facade.login({
        identifier: this.identifier(),
        password: this.password(),
      });

      const returnTo = new URLSearchParams(location.search).get('returnTo');
      await this.router.navigateByUrl(returnTo ?? '/');
    } catch (error) {
      // The API says "invalid credentials" for both a wrong password and an
      // unknown account, on purpose — distinguishing them would let anyone
      // enumerate who holds an account. Passing the message through keeps that
      // property rather than second-guessing it.
      this.error.set(
        error instanceof ApiError ? error.message : 'Could not sign in. Please try again.',
      );
      // A forbidden response here means the address is unconfirmed, and the
      // remedy is a link this page cannot send — so it points at the page that
      // can rather than leaving the message as a dead end.
      this.needsVerification.set(error instanceof ApiError && error.code === 'forbidden');
      this.captchaToken.set('');
    } finally {
      this.busy.set(false);
    }
  }
}
