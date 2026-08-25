import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';

import { AuthFacade } from '../../core/auth/auth-facade';

/**
 * Where a provider sign-in comes back to.
 *
 * The backend has already done the work — exchanged the authorization code,
 * resolved or created the account — and hands the result over as a one-time
 * code in the URL. This page trades that for the normal token pair.
 *
 * **The code expires in 60 seconds and works exactly once**, so the exchange
 * happens immediately on activation rather than behind a button. That design
 * is why tokens are not in the URL: a refresh token in a redirect would land
 * in browser history, server access logs, and the next page's Referer header.
 */
@Component({
  selector: 'app-oauth-callback',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  template: `
    <main class="page callback">
      @if (error()) {
        <h1>Sign-in failed</h1>
        <p class="message">{{ error() }}</p>
        <a routerLink="/login">Back to sign in</a>
      } @else {
        <h1>Signing you in…</h1>
        <p class="message">One moment.</p>
      }
    </main>
  `,
  styles: `
    .callback {
      max-width: 30rem;
      padding-block: var(--space-8);
      text-align: center;
    }
    .message {
      margin: var(--space-4) auto var(--space-5);
      color: var(--ink-soft);
    }
  `,
})
export class OAuthCallback {
  private readonly facade = inject(AuthFacade);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly error = signal<string | null>(null);

  constructor() {
    void this.complete();
  }

  private async complete(): Promise<void> {
    const params = this.route.snapshot.queryParamMap;

    const failure = params.get('error');
    if (failure) {
      this.error.set(describe(failure));
      return;
    }

    const code = params.get('code');
    if (!code) {
      this.error.set('That sign-in link is missing its code. Please try again.');
      return;
    }

    try {
      await this.facade.completeOAuth(code);
      await this.router.navigateByUrl('/');
    } catch {
      // Almost always the 60-second window having elapsed, or a code that was
      // already redeemed — a reload of this page does exactly that.
      this.error.set('That sign-in link has expired or was already used. Please try again.');
    }
  }
}

/**
 * The backend sends deliberately coarse reasons, so a forged flow cannot be
 * probed for which stage it reached. Only `email_taken` is worth explaining,
 * because it is the one the user can actually act on.
 */
function describe(reason: string): string {
  switch (reason) {
    case 'email_taken':
      return 'An account already uses that email address. Sign in with your password, then link the provider from your account settings.';
    case 'access_denied':
      return 'You declined to share your details with ScoutPulse.';
    case 'unknown_provider':
    case 'not_enabled':
      return 'That sign-in method is not available.';
    default:
      return 'Something went wrong signing you in. Please try again.';
  }
}
