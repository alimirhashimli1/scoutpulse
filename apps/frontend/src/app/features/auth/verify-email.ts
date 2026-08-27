import { ChangeDetectionStrategy, Component, inject, input, resource, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';

import { ApiError } from '../../core/api/api-error';
import { AuthRepository } from '../../core/auth/auth-repository';
import { ErrorState, Loading } from '../../shared/ui/states';

/**
 * The page a verification link lands on.
 *
 * The token arrives in the query string because that is the only thing a link
 * in an email can carry — but it is posted back to the API rather than being
 * sent as a GET, so it never reaches an access log or a Referer header as a
 * *credential in a request the server acts on*. It is spent the moment this
 * page loads, which also means a mail client that pre-fetches links consumes
 * it exactly once rather than breaking the second attempt.
 */
@Component({
  selector: 'app-verify-email',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, Loading, ErrorState],
  template: `
    <main class="page auth">
      <h1>Confirm your address</h1>

      @if (!token()) {
        <app-error-state message="This link is missing its token. Open the link from your email." />
      } @else if (result.isLoading()) {
        <app-loading message="Confirming…" />
      } @else if (result.error()) {
        <app-error-state [message]="errorMessage()" />

        <!--
          The failure is nearly always an expired or already-used link, and the
          remedy is the same in every case, so the resend form is offered right
          here rather than sending the visitor away to find it.
        -->
        <section class="resend">
          <h4>Send a new link</h4>
          <form (ngSubmit)="resend()">
            <label for="email">Your email address</label>
            <input
              id="email"
              name="email"
              type="email"
              autocomplete="email"
              required
              [(ngModel)]="email"
            />
            <button class="btn" type="submit" [disabled]="sending()">
              {{ sending() ? 'Sending…' : 'Send a new link' }}
            </button>
          </form>
          @if (sent()) {
            <p class="done" role="status">
              If that address needs confirming, a link is on its way.
            </p>
          }
        </section>
      } @else {
        <p class="done" role="status">Your address is confirmed. You can sign in now.</p>
        <p class="alt"><a class="btn primary" routerLink="/login">Sign in</a></p>
      }
    </main>
  `,
  styles: `
    .auth {
      max-width: 28rem;
      padding-block: var(--space-8);
    }
    h1 {
      margin-bottom: var(--space-5);
    }
    .done {
      color: var(--positive);
      margin-bottom: var(--space-5);
    }
    .resend {
      margin-top: var(--space-6);
      padding-top: var(--space-5);
      border-top: 1px solid var(--line);
    }
    h4 {
      margin-bottom: var(--space-3);
    }
    form {
      display: flex;
      flex-direction: column;
      gap: var(--space-3);
    }
    label {
      font-size: var(--text-xs);
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--muted);
    }
    input {
      padding: var(--space-3);
      border: 1px solid var(--line);
      border-radius: var(--radius);
      background: var(--surface);
      color: var(--ink);
    }
    .alt {
      margin-top: var(--space-5);
    }
  `,
})
export class VerifyEmail {
  private readonly auth = inject(AuthRepository);

  /** Bound from ?token= by withComponentInputBinding(). */
  readonly token = input('');

  protected readonly email = signal('');
  protected readonly sending = signal(false);
  protected readonly sent = signal(false);

  protected readonly result = resource({
    params: () => {
      const token = this.token();
      return token ? { token } : undefined;
    },
    loader: ({ params }) => this.auth.verifyEmail(params.token),
  });

  protected errorMessage(): string {
    const error = this.result.error();
    // The API's message already says the link is invalid or expired and to
    // request a new one, which is more useful than anything generic.
    return error instanceof ApiError
      ? error.message
      : 'That link could not be confirmed. Request a new one.';
  }

  protected async resend(): Promise<void> {
    if (this.sending()) return;

    this.sending.set(true);
    try {
      await this.auth.resendVerification(this.email());
    } catch {
      // Deliberately ignored. The endpoint answers the same way whether or not
      // the address exists, and surfacing a failure here would leak the
      // difference the API works to hide.
    } finally {
      this.sending.set(false);
      this.sent.set(true);
    }
  }
}
