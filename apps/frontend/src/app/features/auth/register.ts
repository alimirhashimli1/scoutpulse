import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  resource,
  signal,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';

import { ApiError } from '../../core/api/api-error';
import { AuthFacade } from '../../core/auth/auth-facade';
import { AuthRepository } from '../../core/auth/auth-repository';
import { Captcha } from '../../shared/forms/captcha';
import { ProviderButtons } from '../../shared/forms/provider-buttons';

/** Mirrors the server's minimum so the failure is immediate, not a round trip. */
const MIN_PASSWORD_LENGTH = 8;

@Component({
  selector: 'app-register',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, Captcha, ProviderButtons],
  template: `
    <main class="page auth">
      <h1>Create an account</h1>

      @if (pendingVerification()) {
        <!--
          The account exists but cannot be used yet, so this is deliberately
          not a redirect: sending someone to a page they are not signed in for
          reads as the registration having failed.
        -->
        <p class="done" role="status">
          Account created. Open the link we sent to <strong>{{ email() }}</strong> to confirm the
          address, then sign in.
        </p>
        <p class="alt">Nothing arrived? <a routerLink="/verify-email">Request another link</a>.</p>
      } @else {
        <form (ngSubmit)="submit()">
          <label for="username">Username</label>
          <input
            id="username"
            name="username"
            autocomplete="username"
            required
            [(ngModel)]="username"
          />

          <label for="email">Email</label>
          <input
            id="email"
            name="email"
            type="email"
            autocomplete="email"
            required
            [(ngModel)]="email"
          />

          <label for="password">Password</label>
          <input
            id="password"
            name="password"
            type="password"
            autocomplete="new-password"
            required
            [(ngModel)]="password"
          />
          <p class="hint">At least {{ minLength }} characters.</p>

          <label for="confirm">Repeat password</label>
          <input
            id="confirm"
            name="confirm"
            type="password"
            autocomplete="new-password"
            required
            [(ngModel)]="confirmPassword"
          />
          @if (mismatch()) {
            <p class="error" role="alert">The two passwords do not match.</p>
          }

          <app-captcha
            [provider]="config.value()?.captcha?.provider ?? ''"
            [siteKey]="config.value()?.captcha?.site_key ?? ''"
            [(token)]="captchaToken"
          />

          @if (error()) {
            <p class="error" role="alert">{{ error() }}</p>
          }

          <button type="submit" [disabled]="busy()">
            {{ busy() ? 'Creating…' : 'Create account' }}
          </button>
        </form>

        <app-provider-buttons />

        <p class="alt">Already have one? <a routerLink="/login">Sign in</a></p>
      }
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
    .hint {
      font-size: var(--text-xs);
      color: var(--muted);
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
    .alt {
      margin-top: var(--space-6);
      font-size: var(--text-sm);
      color: var(--muted);
    }
  `,
})
export class Register {
  private readonly facade = inject(AuthFacade);
  private readonly auth = inject(AuthRepository);
  private readonly router = inject(Router);

  protected readonly minLength = MIN_PASSWORD_LENGTH;

  protected readonly username = signal('');
  protected readonly email = signal('');
  protected readonly password = signal('');
  protected readonly confirmPassword = signal('');
  protected readonly captchaToken = signal('');
  protected readonly busy = signal(false);
  protected readonly error = signal<string | null>(null);
  protected readonly pendingVerification = signal(false);

  /** Whether a challenge is required, and which one, is the server's call. */
  protected readonly config = resource({
    loader: () => this.auth.authConfig(),
  });

  /**
   * Only once something has been typed, so the form does not accuse the
   * visitor of a mismatch before they have reached the second field.
   */
  protected readonly mismatch = computed(
    () => this.confirmPassword().length > 0 && this.confirmPassword() !== this.password(),
  );

  protected async submit(): Promise<void> {
    if (this.busy()) return;

    if (this.password().length < MIN_PASSWORD_LENGTH) {
      this.error.set(`Password must be at least ${MIN_PASSWORD_LENGTH} characters.`);
      return;
    }
    // Checked here as well as in the template, because the template only
    // reports it — nothing stopped a submission with the fields disagreeing.
    if (this.password() !== this.confirmPassword()) {
      this.error.set('The two passwords do not match.');
      return;
    }

    this.busy.set(true);
    this.error.set(null);

    try {
      // Registering never grants a role — the API always creates a plain user
      // and refuses a client-supplied `role` outright.
      const needsVerification = await this.facade.register({
        username: this.username(),
        email: this.email(),
        password: this.password(),
        captcha_token: this.captchaToken() || undefined,
      });

      if (needsVerification) {
        this.pendingVerification.set(true);
        return;
      }
      await this.router.navigateByUrl('/');
    } catch (error) {
      this.error.set(error instanceof ApiError ? error.message : 'Could not create the account.');
      // A challenge token is single use. Leaving a spent one in place makes
      // the next attempt fail on the challenge rather than on whatever the
      // visitor actually needs to correct.
      this.captchaToken.set('');
    } finally {
      this.busy.set(false);
    }
  }
}
