import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  resource,
  signal,
} from '@angular/core';
import { DatePipe } from '@angular/common';
import { FormsModule } from '@angular/forms';

import { PageQuery } from '../../core/api/page';
import { USER_ADMIN_READER, USER_ADMIN_WRITER } from '../../core/api/user-admin';
import { SessionStore } from '../../core/auth/session-store';
import { Role, User } from '../../core/models/identity';
import { Paginator } from '../../shared/pagination/paginator';
import { messageFor } from '../../shared/forms/submit';
import { Empty, ErrorState, Loading } from '../../shared/ui/states';

const ROLES: Role[] = ['user', 'editor', 'admin'];

/**
 * Accounts, and what each one may do.
 *
 * Two API behaviours drive the interaction here, and both are stated in the UI
 * rather than left to surprise someone:
 *
 * **Changing a role signs that person out everywhere.** The role travels
 * inside the access token, so a demotion that did not revoke sessions would
 * leave the old privileges live until the refresh token expired — up to a day.
 * The service revokes them instead, which is correct and abrupt, so the
 * confirmation says so.
 *
 * **An administrator cannot delete their own account.** The API refuses it, so
 * the control is not offered on your own row; the alternative is a button
 * whose only outcome is an error.
 */
@Component({
  selector: 'app-user-list',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, DatePipe, Paginator, Loading, Empty, ErrorState],
  template: `
    <main class="page">
      <header class="masthead">
        <div>
          <h1>Users</h1>
          <p class="standfirst">Accounts and the roles they hold.</p>
        </div>
      </header>

      <form class="search" role="search" (ngSubmit)="applySearch()">
        <label class="visually-hidden" for="q">Search by username or email</label>
        <input id="q" name="q" type="search" placeholder="Username or email…" [(ngModel)]="query" />
        <button class="btn" type="submit">Search</button>
        @if (activeQuery()) {
          <button class="btn" type="button" (click)="clearSearch()">Clear</button>
        }
      </form>

      @if (actionError()) {
        <app-error-state [message]="actionError()!" />
      }

      @if (users.isLoading()) {
        <app-loading message="Loading users…" />
      } @else if (users.error()) {
        <app-error-state [message]="errorMessage()" />
      } @else if (!users.value()?.items?.length) {
        <app-empty [message]="activeQuery() ? 'Nobody matches that.' : 'No accounts yet.'" />
      } @else {
        <div class="scroll-x">
          <table>
            <thead>
              <tr>
                <th scope="col">Username</th>
                <th scope="col">Email</th>
                <th scope="col">Role</th>
                <th scope="col">Joined</th>
                <th scope="col"><span class="visually-hidden">Actions</span></th>
              </tr>
            </thead>
            <tbody>
              @for (user of users.value()!.items; track user.id) {
                <tr [class.self]="isSelf(user)">
                  <td>
                    {{ user.username }}
                    @if (isSelf(user)) {
                      <span class="you">you</span>
                    }
                  </td>
                  <td class="muted">{{ user.email }}</td>
                  <td>
                    <label class="visually-hidden" [attr.for]="'role-' + user.id">
                      Role for {{ user.username }}
                    </label>
                    <select
                      [id]="'role-' + user.id"
                      [value]="user.role"
                      [disabled]="busyWith() === user.id"
                      (change)="changeRole(user, $event)"
                    >
                      @for (role of roles; track role) {
                        <option [value]="role">{{ role }}</option>
                      }
                    </select>
                  </td>
                  <td class="muted tabular">{{ user.created_at | date: 'd MMM y' }}</td>
                  <td class="right">
                    <!--
                      Absent on your own row: the API refuses it, so offering
                      the button would only produce an error message.
                    -->
                    @if (!isSelf(user)) {
                      <button
                        class="btn danger"
                        type="button"
                        [disabled]="busyWith() === user.id"
                        (click)="remove(user)"
                      >
                        Delete
                      </button>
                    }
                  </td>
                </tr>
              }
            </tbody>
          </table>
        </div>

        <app-paginator [page]="users.value()!" label="accounts" (pageChange)="goTo($event)" />
      }
    </main>
  `,
  styles: `
    .masthead {
      padding-block: var(--space-6) var(--space-4);
    }
    h1 {
      margin-bottom: var(--space-2);
    }
    .standfirst {
      color: var(--ink-soft);
    }
    .search {
      display: flex;
      gap: var(--space-2);
      margin-bottom: var(--space-5);
      flex-wrap: wrap;
    }
    .search input {
      flex: 1 1 16rem;
      padding: var(--space-2) var(--space-3);
      border: 1px solid var(--line);
      border-radius: var(--radius);
      background: var(--surface);
    }
    table {
      width: 100%;
      border-collapse: collapse;
      font-size: var(--text-sm);
    }
    th {
      text-align: left;
      font-size: var(--text-xs);
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--muted);
      font-weight: 700;
      padding: var(--space-3);
      background: var(--surface-2);
      white-space: nowrap;
    }
    td {
      padding: var(--space-3);
      border-bottom: 1px solid var(--line-soft);
      vertical-align: middle;
    }
    td select {
      padding: var(--space-1) var(--space-2);
      border: 1px solid var(--line);
      border-radius: var(--radius);
      background: var(--surface);
    }
    .self {
      background: var(--accent-soft);
    }
    .you {
      font-family: var(--font-mono);
      font-size: 10px;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--accent);
      border: 1px solid var(--accent);
      border-radius: var(--radius-sm);
      padding: 1px 5px;
      margin-left: var(--space-2);
    }
    .right {
      text-align: right;
    }
    .muted {
      color: var(--muted);
    }
  `,
})
export class UserList {
  private readonly reader = inject(USER_ADMIN_READER);
  private readonly writer = inject(USER_ADMIN_WRITER);
  private readonly session = inject(SessionStore);

  protected readonly roles = ROLES;

  protected readonly query = signal('');
  private readonly activeQueryValue = signal('');
  protected readonly activeQuery = this.activeQueryValue.asReadonly();
  private readonly page = signal<PageQuery>({ limit: 25, offset: 0 });

  protected readonly busyWith = signal<string | null>(null);
  protected readonly actionError = signal<string | null>(null);

  protected readonly users = resource({
    params: () => ({ q: this.activeQueryValue(), page: this.page() }),
    loader: ({ params }) => this.reader.list({ ...params.page, q: params.q || undefined }),
  });

  protected readonly errorMessage = computed(() =>
    messageFor(this.users.error(), 'Could not load accounts.'),
  );

  protected isSelf(user: User): boolean {
    return user.id === this.session.user()?.id;
  }

  protected applySearch(): void {
    this.page.set({ limit: 25, offset: 0 });
    this.activeQueryValue.set(this.query().trim());
  }

  protected clearSearch(): void {
    this.query.set('');
    this.applySearch();
  }

  protected goTo(query: PageQuery): void {
    this.page.set(query);
  }

  protected async changeRole(user: User, event: Event): Promise<void> {
    const select = event.target as HTMLSelectElement;
    const role = select.value as Role;
    if (role === user.role) return;

    const consequence = this.isSelf(user)
      ? `\n\nThis is your own account. You will be signed out immediately, and you may not be able to undo it.`
      : `\n\n${user.username} will be signed out of every device and will have to sign in again.`;

    if (!confirm(`Change ${user.username} from ${user.role} to ${role}?${consequence}`)) {
      // Put the control back: the model did not change, so nothing else would.
      select.value = user.role;
      return;
    }

    this.busyWith.set(user.id);
    this.actionError.set(null);

    try {
      await this.writer.setRole(user.id, role);
      this.users.reload();
    } catch (error) {
      select.value = user.role;
      this.actionError.set(messageFor(error, 'Could not change the role.'));
    } finally {
      this.busyWith.set(null);
    }
  }

  protected async remove(user: User): Promise<void> {
    if (!confirm(`Delete ${user.username}? Their sessions end at once. This cannot be undone.`)) {
      return;
    }

    this.busyWith.set(user.id);
    this.actionError.set(null);

    try {
      await this.writer.remove(user.id);
      this.users.reload();
    } catch (error) {
      this.actionError.set(messageFor(error, 'Could not delete the account.'));
    } finally {
      this.busyWith.set(null);
    }
  }
}
