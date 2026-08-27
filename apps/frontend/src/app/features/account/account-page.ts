import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  resource,
  signal,
} from '@angular/core';
import { DatePipe, TitleCasePipe } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';

import { AuthRepository } from '../../core/auth/auth-repository';
import { Permissions } from '../../core/auth/permissions';
import { SessionStore } from '../../core/auth/session-store';
import { Field } from '../../shared/forms/field';
import { messageFor } from '../../shared/forms/submit';
import { ErrorState, Loading } from '../../shared/ui/states';

const MIN_PASSWORD = 8;

/**
 * The account page: who you are, your password, and your linked sign-ins.
 *
 * Two things here are shaped by the API rather than by preference.
 *
 * **The password form is always rendered, even for accounts that have no
 * password.** An account created through Google has an empty password hash and
 * `PUT /users/me/password` refuses it — but `GET /users/me` does not report
 * whether a password exists, so the page cannot know in advance. The API's
 * message says exactly what is wrong, so it is shown rather than guessed at.
 * A `has_password` flag on the user would let this be answered before the
 * attempt; that is recorded in ISSUES.md rather than worked around here.
 *
 * **Unlinking can be refused.** The last way into an account may not be
 * removed — no password and no other provider means unlinking would lock the
 * owner out, and there is no password reset to recover with.
 */
@Component({
  selector: 'app-account-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, DatePipe, TitleCasePipe, Field, Loading, ErrorState],
  template: `
    <main class="page account">
      <header class="head">
        <p class="eyebrow">Account</p>
        <h1>{{ session.user()?.username }}</h1>
      </header>

      <section>
        <h4>Profile</h4>
        @if (session.user(); as user) {
          <dl class="facts">
            <div>
              <dt>Username</dt>
              <dd>{{ user.username }}</dd>
            </div>
            <div>
              <dt>Email</dt>
              <dd>{{ user.email }}</dd>
            </div>
            <div>
              <dt>Role</dt>
              <dd>
                {{ user.role | titlecase }}
                @if (user.role === 'editor') {
                  <span class="detail">{{ grantSummary() }}</span>
                }
              </dd>
            </div>
            <div>
              <dt>Member since</dt>
              <dd>{{ user.created_at | date: 'd MMM y' }}</dd>
            </div>
          </dl>
        }
      </section>

      <section>
        <h4>Password</h4>
        <form class="stack" (ngSubmit)="changePassword()" novalidate>
          <app-field for="current" label="Current password">
            <input
              id="current"
              name="current"
              type="password"
              autocomplete="current-password"
              [(ngModel)]="currentPassword"
            />
          </app-field>

          <app-field
            for="next"
            label="New password"
            [hint]="'At least ' + minPassword + ' characters.'"
            [error]="passwordFieldError()"
          >
            <input
              id="next"
              name="next"
              type="password"
              autocomplete="new-password"
              [(ngModel)]="newPassword"
            />
          </app-field>

          @if (passwordError()) {
            <app-error-state [message]="passwordError()!" />
          }
          @if (passwordDone()) {
            <p class="done" role="status">Password changed.</p>
          }

          <div>
            <button class="btn primary" type="submit" [disabled]="changingPassword()">
              {{ changingPassword() ? 'Changing…' : 'Change password' }}
            </button>
          </div>
        </form>
      </section>

      <section>
        <h4>Linked sign-ins</h4>

        @if (identities.isLoading()) {
          <app-loading />
        } @else if (identities.error()) {
          <app-error-state [message]="identitiesErrorMessage()" />
        } @else {
          @if (identities.value()?.length) {
            <ul class="identities">
              @for (identity of identities.value()!; track identity.provider) {
                <li>
                  <span class="provider">{{ identity.provider | titlecase }}</span>
                  @if (identity.email) {
                    <span class="muted">{{ identity.email }}</span>
                  }
                  <span class="since muted">
                    linked {{ identity.created_at | date: 'd MMM y' }}
                  </span>
                  <button
                    class="btn danger"
                    type="button"
                    [disabled]="unlinking() === identity.provider"
                    (click)="unlink(identity.provider)"
                  >
                    {{ unlinking() === identity.provider ? 'Unlinking…' : 'Unlink' }}
                  </button>
                </li>
              }
            </ul>
          } @else {
            <p class="muted">No providers linked. You sign in with your password.</p>
          }

          @if (unlinkError()) {
            <app-error-state [message]="unlinkError()!" />
          }

          <p class="hint">
            Adding a provider is done by signing in with it — do that from the
            <a routerLink="/login">sign-in page</a> while signed out, using the same email address.
          </p>
        }
      </section>
    </main>
  `,
  styles: `
    .account {
      max-width: 42rem;
      padding-block: var(--space-6) var(--space-8);
    }
    .head {
      margin-bottom: var(--space-6);
    }
    .eyebrow {
      font-family: var(--font-mono);
      font-size: var(--text-xs);
      letter-spacing: 0.12em;
      text-transform: uppercase;
      color: var(--muted);
      margin-bottom: var(--space-2);
    }
    h1 {
      font-size: var(--text-2xl);
    }
    section {
      padding-block: var(--space-5);
      border-top: 1px solid var(--line);
    }
    h4 {
      margin-bottom: var(--space-4);
    }
    .facts {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(11rem, 1fr));
      gap: var(--space-4);
      margin: 0;
    }
    dt {
      font-size: var(--text-xs);
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--muted);
      margin-bottom: var(--space-1);
    }
    dd {
      margin: 0;
      font-weight: 600;
    }
    .detail {
      display: block;
      font-weight: 400;
      font-size: var(--text-sm);
      color: var(--muted);
    }
    .stack {
      display: flex;
      flex-direction: column;
      gap: var(--space-4);
      max-width: 22rem;
    }
    .done {
      color: var(--positive);
      font-size: var(--text-sm);
    }
    .identities {
      list-style: none;
      margin: 0 0 var(--space-4);
      padding: 0;
    }
    .identities li {
      display: flex;
      gap: var(--space-3);
      align-items: center;
      flex-wrap: wrap;
      padding: var(--space-3) 0;
      border-bottom: 1px solid var(--line-soft);
    }
    .provider {
      font-weight: 600;
    }
    .since {
      margin-left: auto;
      font-size: var(--text-sm);
    }
    .muted {
      color: var(--muted);
    }
    .hint {
      font-size: var(--text-sm);
      color: var(--muted);
    }
  `,
})
export class AccountPage {
  private readonly auth = inject(AuthRepository);
  private readonly permissions = inject(Permissions);
  protected readonly session = inject(SessionStore);

  protected readonly minPassword = MIN_PASSWORD;

  protected readonly currentPassword = signal('');
  protected readonly newPassword = signal('');
  protected readonly changingPassword = signal(false);
  protected readonly passwordError = signal<string | null>(null);
  protected readonly passwordFieldError = signal<string | null>(null);
  protected readonly passwordDone = signal(false);

  protected readonly unlinking = signal<string | null>(null);
  protected readonly unlinkError = signal<string | null>(null);

  protected readonly identities = resource({
    loader: () => this.auth.identities(),
  });

  protected readonly identitiesErrorMessage = computed(() =>
    messageFor(this.identities.error(), 'Could not load linked sign-ins.'),
  );

  /** How many clubs an editor holds. Answers "what does 'editor' mean for me?" */
  protected readonly grantSummary = computed(() => {
    const count = this.permissions.grantedTeamIds().length;
    if (count === 0) return 'No clubs assigned yet.';
    return count === 1 ? 'Editor of 1 club.' : `Editor of ${count} clubs.`;
  });

  protected async changePassword(): Promise<void> {
    if (this.changingPassword()) return;

    this.passwordDone.set(false);
    this.passwordError.set(null);

    if (this.newPassword().length < MIN_PASSWORD) {
      this.passwordFieldError.set(`At least ${MIN_PASSWORD} characters.`);
      return;
    }
    if (this.newPassword() === this.currentPassword()) {
      this.passwordFieldError.set('Must differ from the current password.');
      return;
    }
    this.passwordFieldError.set(null);

    this.changingPassword.set(true);
    try {
      await this.auth.changePassword(this.currentPassword(), this.newPassword());
      this.currentPassword.set('');
      this.newPassword.set('');
      this.passwordDone.set(true);
    } catch (error) {
      // Covers both "current password is incorrect" and the OAuth-only case,
      // where the account has no password to change at all. Both messages are
      // written to be read by the person who hit them.
      this.passwordError.set(messageFor(error, 'Could not change the password.'));
    } finally {
      this.changingPassword.set(false);
    }
  }

  protected async unlink(provider: string): Promise<void> {
    if (this.unlinking()) return;

    this.unlinking.set(provider);
    this.unlinkError.set(null);

    try {
      await this.auth.unlinkIdentity(provider);
      this.identities.reload();
    } catch (error) {
      // The expected failure is the last-credential guard, whose message
      // explains what to do first.
      this.unlinkError.set(messageFor(error, 'Could not unlink that provider.'));
    } finally {
      this.unlinking.set(null);
    }
  }
}
