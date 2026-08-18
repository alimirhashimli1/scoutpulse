import { InjectionToken } from '@angular/core';

import { Role, User } from '../models/identity';
import { Page, PageQuery } from './page';

/**
 * Administering accounts.
 *
 * Deliberately **not** part of `AuthRepository`, which every page touches to
 * sign in and read the session. Folding "delete any user" into the same object
 * would mean the login form injects something that can delete accounts — the
 * interface-segregation point from §4, with unusually high stakes.
 */

export interface UserFilter extends PageQuery {
  /** Matches username or email. Absent lists everyone. */
  q?: string;
}

export interface UserAdminReader {
  list(filter?: UserFilter): Promise<Page<User>>;
}

export interface UserAdminWriter {
  /**
   * Changes a role.
   *
   * **This signs the target out everywhere.** The old role is baked into any
   * token already issued, so the service revokes their sessions to force a
   * fresh one — otherwise a demoted administrator would keep their privileges
   * until their refresh token expired, up to 24 hours later.
   */
  setRole(userId: string, role: Role): Promise<void>;
  /** Refused for the caller's own account, with a message saying so. */
  remove(userId: string): Promise<void>;
}

export const USER_ADMIN_READER = new InjectionToken<UserAdminReader>('UserAdminReader');
export const USER_ADMIN_WRITER = new InjectionToken<UserAdminWriter>('UserAdminWriter');
