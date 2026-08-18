import { Injectable } from '@angular/core';

import { Role, User } from '../../models/identity';
import { Page } from '../page';
import { UserAdminReader, UserAdminWriter, UserFilter } from '../user-admin';
import { HttpRepository } from './http-repository';

/**
 * Account administration against identity-svc.
 *
 * The base differs from every other repository here — `api.identity`, not
 * `api.football` — which is the whole reason `HttpRepository` takes the base
 * as an abstract member rather than hardcoding one.
 */
@Injectable()
export class HttpUserAdminRepository
  extends HttpRepository
  implements UserAdminReader, UserAdminWriter
{
  protected readonly base = this.api.identity;

  list(filter?: UserFilter): Promise<Page<User>> {
    return this.getPage<User>(this.url('users'), filter);
  }

  async setRole(userId: string, role: Role): Promise<void> {
    await this.put<{ id: string; role: Role }>(this.url('users', userId, 'role'), { role });
  }

  remove(userId: string): Promise<void> {
    return this.deleteAt(this.url('users', userId));
  }
}
