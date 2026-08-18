import { Injectable, computed, effect, inject, signal } from '@angular/core';

import { TEAM_EDITOR_READER } from '../api/contracts';
import { SessionStore } from './session-store';

/**
 * What the signed-in user may write, mirroring the server's rules.
 *
 * Not a security boundary — the API re-checks every one of these, and
 * everything consulted here is client-side state. It exists so the UI offers
 * the actions that will actually succeed. An "Edit" button that always answers
 * with a 403 teaches people to distrust the interface.
 *
 * The rules come straight from `Authorizer` in football-svc, and are worth
 * stating because they are not the obvious ones:
 *
 * - An **administrator** may do anything.
 * - An **editor** may act on a club they hold a grant for.
 * - A record belonging to **no club** — a free agent, an unattached coach — is
 *   administrator-only, because no club grant can cover it. This is why an
 *   editor cannot create a player without naming their club.
 * - A **transfer** is authorised by *either* end. The selling club releasing a
 *   player and the buying club signing one are the same event, so either
 *   club's editor may file it.
 *
 * Grants live in the database rather than the token, so they are read once per
 * session from `GET /users/{id}/teams` — the endpoint that replaced the
 * `managed_team_ids` claim, precisely so a revocation takes effect at once
 * instead of whenever the token happened to expire.
 */
@Injectable({ providedIn: 'root' })
export class Permissions {
  private readonly session = inject(SessionStore);
  private readonly editors = inject(TEAM_EDITOR_READER);

  private readonly grants = signal<ReadonlySet<string>>(new Set());
  private readonly loaded = signal(false);

  readonly isAdmin = this.session.isAdmin;
  readonly isEditor = this.session.isEditor;

  /** The clubs this user holds a grant for. Empty for an administrator, who needs none. */
  readonly grantedTeamIds = computed(() => [...this.grants()]);

  /** True once grants have been fetched, so a form can wait rather than flash. */
  readonly ready = computed(() => this.session.isAdmin() || this.loaded());

  constructor() {
    // Grants follow the session: loaded when someone signs in, dropped when
    // they sign out. Doing it here rather than in each form means a page never
    // renders against a previous user's permissions.
    effect(() => {
      const user = this.session.user();

      if (!user) {
        this.grants.set(new Set());
        this.loaded.set(false);
        return;
      }

      // An administrator's grants are irrelevant — they are permitted
      // everywhere — so the request is skipped rather than made and ignored.
      if (user.role === 'admin') {
        this.loaded.set(true);
        return;
      }

      void this.load(user.id);
    });
  }

  private async load(userId: string): Promise<void> {
    try {
      const ids = await this.editors.teamsFor(userId);
      this.grants.set(new Set(ids));
    } catch {
      // A failed grant lookup means no known grants, which hides write
      // controls rather than offering ones that would 403. The API stays the
      // authority either way.
      this.grants.set(new Set());
    } finally {
      this.loaded.set(true);
    }
  }

  /**
   * `RequireTargetTeam`: an administrator, or an editor holding this club.
   * A null club is administrator-only — no grant can cover "no club".
   */
  canEditTeam(teamId: string | null | undefined): boolean {
    if (this.session.isAdmin()) return true;
    if (!this.session.isEditor()) return false;
    if (!teamId) return false;
    return this.grants().has(teamId);
  }

  /**
   * `RequireEitherTeam`: either end of a move authorises it.
   *
   * Both ends null cannot happen — the API rejects a transfer with neither
   * origin nor destination — but an editor holding neither club is refused,
   * which is the case this returns false for.
   */
  canMoveBetween(from: string | null | undefined, to: string | null | undefined): boolean {
    if (this.session.isAdmin()) return true;
    if (!this.session.isEditor()) return false;

    const held = this.grants();
    return (!!from && held.has(from)) || (!!to && held.has(to));
  }

  /** Editing a player's descriptive fields is checked against their club. */
  canEditPlayer(player: { team_id: string | null }): boolean {
    return this.canEditTeam(player.team_id);
  }

  /** Deleting anything, and every league, season and valuation, is admin-only. */
  canAdminister(): boolean {
    return this.session.isAdmin();
  }
}
