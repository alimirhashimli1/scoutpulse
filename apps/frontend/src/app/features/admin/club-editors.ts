import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  input,
  resource,
  signal,
} from '@angular/core';
import { DatePipe } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';

import { TEAM_EDITOR_READER, TEAM_EDITOR_WRITER, TEAM_READER } from '../../core/api/contracts';
import { USER_ADMIN_READER } from '../../core/api/user-admin';
import { SessionStore } from '../../core/auth/session-store';
import { User } from '../../core/models/identity';
import { messageFor } from '../../shared/forms/submit';
import { Empty, ErrorState, Loading } from '../../shared/ui/states';

/**
 * Who may edit a club.
 *
 * A grant is narrow on purpose: it lets someone maintain a club's squad,
 * transfers and competition entries. It does **not** let them rename the club,
 * value a player, or grant access to anyone else — an editor who could hand
 * out their own club's access would make the boundary meaningless.
 *
 * Grants live in the football service's database and are resolved per request,
 * so both granting and revoking take effect immediately. They used to travel
 * inside the JWT, where a revocation could not apply until the token expired.
 *
 * **Names are resolved client-side.** `GET /teams/{id}/editors` returns user
 * ids, and identity-svc has no endpoint to fetch a user by id — only a paged
 * list. So a page of accounts is loaded and matched locally, and an unresolved
 * id renders as itself rather than as blank. That is adequate for a few
 * hundred accounts and is the same trade `LookupStore` makes for clubs; past
 * that it wants either a batch lookup or usernames on the grant row.
 */
@Component({
  selector: 'app-club-editors',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, DatePipe, Loading, Empty, ErrorState],
  template: `
    <main class="page editors">
      <header class="head">
        <p class="eyebrow">Editor access</p>
        <h1>{{ club.value()?.name ?? 'Club' }}</h1>
        <p class="standfirst">
          Editors maintain this club's squad, transfers and competition entries. They cannot rename
          the club, record valuations, or grant access to anyone else.
        </p>
        <p><a routerLink="/clubs/{{ id() }}">Back to the club</a></p>
      </header>

      @if (actionError()) {
        <app-error-state [message]="actionError()!" />
      }

      <section>
        <h4>Current editors</h4>

        @if (grants.isLoading()) {
          <app-loading />
        } @else if (grants.error()) {
          <app-error-state [message]="grantsErrorMessage()" />
        } @else if (!grants.value()?.items?.length) {
          <app-empty
            message="Nobody yet."
            hint="Only administrators can maintain this club until someone is granted access."
          />
        } @else {
          <ul class="list">
            @for (grant of grants.value()!.items; track grant.user_id) {
              <li>
                <span class="who">{{ nameFor(grant.user_id) }}</span>
                <span class="since muted"> granted {{ grant.granted_at | date: 'd MMM y' }} </span>
                <button
                  class="btn danger"
                  type="button"
                  [disabled]="busyWith() === grant.user_id"
                  (click)="revoke(grant.user_id)"
                >
                  {{ busyWith() === grant.user_id ? 'Revoking…' : 'Revoke' }}
                </button>
              </li>
            }
          </ul>
        }
      </section>

      <section>
        <h4>Grant access</h4>
        <form class="search" role="search" (ngSubmit)="applySearch()">
          <label class="visually-hidden" for="q">Find an account</label>
          <input
            id="q"
            name="q"
            type="search"
            placeholder="Username or email…"
            [(ngModel)]="query"
          />
          <button class="btn" type="submit">Find</button>
        </form>

        @if (activeQuery()) {
          @if (candidates.isLoading()) {
            <app-loading />
          } @else if (!candidates.value()?.items?.length) {
            <app-empty message="Nobody matches that." />
          } @else {
            <ul class="list">
              @for (user of candidates.value()!.items; track user.id) {
                <li>
                  <span class="who">{{ user.username }}</span>
                  <span class="muted">{{ user.email }}</span>
                  <span class="role muted">{{ user.role }}</span>
                  @if (alreadyGranted(user.id)) {
                    <span class="granted">already an editor</span>
                  } @else {
                    <button
                      class="btn"
                      type="button"
                      [disabled]="busyWith() === user.id"
                      (click)="grant(user)"
                    >
                      {{ busyWith() === user.id ? 'Granting…' : 'Grant' }}
                    </button>
                  }
                </li>
              }
            </ul>
            <p class="hint">
              A grant only takes effect for accounts with the <strong>editor</strong> or
              <strong>admin</strong> role. Granting a plain user changes nothing until their role is
              raised on the <a routerLink="/admin/users">users page</a>.
            </p>
          }
        }
      </section>
    </main>
  `,
  styles: `
    .editors {
      max-width: 46rem;
      padding-block: var(--space-6) var(--space-8);
    }
    .head {
      margin-bottom: var(--space-5);
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
      margin-bottom: var(--space-3);
    }
    .standfirst {
      color: var(--ink-soft);
      margin-bottom: var(--space-3);
    }
    section {
      padding-block: var(--space-5);
      border-top: 1px solid var(--line);
    }
    h4 {
      margin-bottom: var(--space-4);
    }
    .list {
      list-style: none;
      margin: 0;
      padding: 0;
    }
    .list li {
      display: flex;
      gap: var(--space-3);
      align-items: center;
      flex-wrap: wrap;
      padding: var(--space-3) 0;
      border-bottom: 1px solid var(--line-soft);
    }
    .who {
      font-weight: 600;
    }
    .role {
      font-family: var(--font-mono);
      font-size: 11px;
    }
    .since {
      margin-left: auto;
      font-size: var(--text-sm);
    }
    .granted {
      margin-left: auto;
      font-size: var(--text-sm);
      color: var(--muted);
    }
    .search {
      display: flex;
      gap: var(--space-2);
      margin-bottom: var(--space-4);
      flex-wrap: wrap;
    }
    .search input {
      flex: 1 1 16rem;
      padding: var(--space-2) var(--space-3);
      border: 1px solid var(--line);
      border-radius: var(--radius);
      background: var(--surface);
    }
    .hint {
      font-size: var(--text-sm);
      color: var(--muted);
      margin-top: var(--space-3);
    }
    .muted {
      color: var(--muted);
    }
  `,
})
export class ClubEditors {
  private readonly teams = inject(TEAM_READER);
  private readonly editorReader = inject(TEAM_EDITOR_READER);
  private readonly editorWriter = inject(TEAM_EDITOR_WRITER);
  private readonly users = inject(USER_ADMIN_READER);
  private readonly session = inject(SessionStore);

  readonly id = input.required<string>();

  protected readonly query = signal('');
  private readonly activeQueryValue = signal('');
  protected readonly activeQuery = this.activeQueryValue.asReadonly();

  protected readonly busyWith = signal<string | null>(null);
  protected readonly actionError = signal<string | null>(null);

  protected readonly club = resource({
    params: () => ({ id: this.id() }),
    loader: ({ params }) => this.teams.byId(params.id),
  });

  protected readonly grants = resource({
    params: () => ({ id: this.id() }),
    loader: ({ params }) => this.editorReader.editors(params.id, { limit: 100 }),
  });

  /** A page of accounts, purely to turn the ids on grant rows into names. */
  private readonly directory = resource({
    loader: () => this.users.list({ limit: 100 }),
  });

  protected readonly candidates = resource({
    params: () => {
      const q = this.activeQueryValue();
      return q ? { q } : undefined;
    },
    loader: ({ params }) => this.users.list({ q: params.q, limit: 25 }),
  });

  protected readonly grantsErrorMessage = computed(() =>
    messageFor(this.grants.error(), 'Could not load editors.'),
  );

  private readonly namesById = computed(() => {
    const map = new Map<string, string>();
    for (const user of this.directory.value()?.items ?? []) map.set(user.id, user.username);
    // The signed-in administrator is always resolvable, even if they fall
    // outside the page of accounts that was loaded.
    const me = this.session.user();
    if (me) map.set(me.id, me.username);
    return map;
  });

  protected nameFor(userId: string): string {
    return this.namesById().get(userId) ?? userId;
  }

  protected alreadyGranted(userId: string): boolean {
    return (this.grants.value()?.items ?? []).some((grant) => grant.user_id === userId);
  }

  protected applySearch(): void {
    this.activeQueryValue.set(this.query().trim());
  }

  protected async grant(user: User): Promise<void> {
    this.busyWith.set(user.id);
    this.actionError.set(null);

    try {
      await this.editorWriter.grant(this.id(), user.id);
      this.grants.reload();
      this.directory.reload();
    } catch (error) {
      this.actionError.set(messageFor(error, 'Could not grant access.'));
    } finally {
      this.busyWith.set(null);
    }
  }

  protected async revoke(userId: string): Promise<void> {
    if (!confirm(`Revoke ${this.nameFor(userId)}'s access to this club? It applies immediately.`)) {
      return;
    }

    this.busyWith.set(userId);
    this.actionError.set(null);

    try {
      await this.editorWriter.revoke(this.id(), userId);
      this.grants.reload();
    } catch (error) {
      this.actionError.set(messageFor(error, 'Could not revoke access.'));
    } finally {
      this.busyWith.set(null);
    }
  }
}
