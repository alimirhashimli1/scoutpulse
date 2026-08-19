import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';

import { ApiError } from '../../core/api/api-error';
import { AuthFacade } from '../../core/auth/auth-facade';

/** Mirrors the server's minimum so the failure is immediate, not a round trip. */
const MIN_PASSWORD_LENGTH = 8;

@Component({
  selector: 'app-register',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink],
  template: `
    <main class="page auth">
      <h1>Create an account</h1>

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

        @if (error()) {
          <p class="error" role="alert">{{ error() }}</p>
        }

        <button type="submit" [disabled]="busy()">
          {{ busy() ? 'Creating…' : 'Create account' }}
        </button>
      </form>

      <p class="alt">Already have one? <a routerLink="/login">Sign in</a></p>
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
  private readonly router = inject(Router);

  protected readonly minLength = MIN_PASSWORD_LENGTH;

  protected readonly username = signal('');
  protected readonly email = signal('');
  protected readonly password = signal('');
  protected readonly busy = signal(false);
  protected readonly error = signal<string | null>(null);

  protected async submit(): Promise<void> {
    if (this.busy()) return;

    if (this.password().length < MIN_PASSWORD_LENGTH) {
      this.error.set(`Password must be at least ${MIN_PASSWORD_LENGTH} characters.`);
      return;
    }

    this.busy.set(true);
    this.error.set(null);

    try {
      // Registering never grants a role — the API always creates a plain user
      // and refuses a client-supplied `role` outright.
      await this.facade.register({
        username: this.username(),
        email: this.email(),
        password: this.password(),
      });
      await this.router.navigateByUrl('/');
    } catch (error) {
      this.error.set(error instanceof ApiError ? error.message : 'Could not create the account.');
    } finally {
      this.busy.set(false);
    }
  }
}
