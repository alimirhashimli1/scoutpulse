import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  resource,
  signal,
} from '@angular/core';
import { RouterLink } from '@angular/router';

import { TEAM_READER } from '../../core/api/contracts';
import { LookupStore } from '../../core/api/lookup-store';
import { PageQuery } from '../../core/api/page';
import { Permissions } from '../../core/auth/permissions';
import { Seo } from '../../core/seo/seo';
import { Paginator } from '../../shared/pagination/paginator';
import { Empty, ErrorState, Loading } from '../../shared/ui/states';

/**
 * Every club.
 *
 * This page could not exist until the API made `league_id` optional — it used
 * to be mandatory, so there was no way to ask for all clubs at all.
 */
@Component({
  selector: 'app-club-list',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, Paginator, Loading, Empty, ErrorState],
  template: `
    <main class="page">
      <header class="masthead">
        <div>
          <h1>Clubs</h1>
          <p class="standfirst">Every club on record.</p>
        </div>
        @if (permissions.canAdminister()) {
          <a class="btn primary" routerLink="/clubs/new">New club</a>
        }
      </header>

      @if (clubs.isLoading()) {
        <app-loading message="Loading clubs…" />
      } @else if (clubs.error()) {
        <app-error-state [message]="errorMessage()" />
      } @else if (!clubs.value()?.items?.length) {
        <app-empty message="No clubs yet." hint="Create one and it will appear here." />
      } @else {
        <ul class="clubs">
          @for (club of clubs.value()!.items; track club.id) {
            <li>
              <a [routerLink]="['/clubs', club.id]">
                <span class="name">{{ club.name }}</span>
                <span class="meta">
                  {{ lookup.leagueName(club.league_id, '') }}
                  @if (club.city) {
                    <span class="muted">· {{ club.city }}</span>
                  }
                </span>
              </a>
            </li>
          }
        </ul>

        <app-paginator [page]="clubs.value()!" label="club results" (pageChange)="goTo($event)" />
      }
    </main>
  `,
  styles: `
    .masthead {
      display: flex;
      justify-content: space-between;
      align-items: flex-end;
      gap: var(--space-4);
      flex-wrap: wrap;
      padding-block: var(--space-6) var(--space-5);
    }
    h1 {
      margin-bottom: var(--space-2);
    }
    .standfirst {
      color: var(--ink-soft);
    }
    .clubs {
      list-style: none;
      margin: 0;
      padding: 0;
    }
    .clubs li {
      border-bottom: 1px solid var(--line-soft);
    }
    .clubs a {
      display: flex;
      justify-content: space-between;
      gap: var(--space-4);
      align-items: baseline;
      padding: var(--space-3) 0;
      text-decoration: none;
      color: var(--ink);
    }
    .clubs a:hover .name {
      color: var(--accent);
    }
    .name {
      font-weight: 600;
    }
    .meta {
      color: var(--ink-soft);
      font-size: var(--text-sm);
    }
    .muted {
      color: var(--muted);
    }
  `,
})
export class ClubList {
  private readonly reader = inject(TEAM_READER);
  protected readonly lookup = inject(LookupStore);
  protected readonly permissions = inject(Permissions);
  private readonly seo = inject(Seo);

  private readonly page = signal<PageQuery>({ limit: 25, offset: 0 });

  constructor() {
    this.seo.describe({
      title: 'Clubs',
      description: 'Every club on record, with squads, managerial history and transfers.',
      path: '/clubs',
    });
  }

  protected readonly clubs = resource({
    params: () => ({ page: this.page() }),
    loader: async ({ params }) => {
      await this.lookup.loadLeagues();
      return this.reader.list(params.page);
    },
  });

  protected readonly errorMessage = computed(() => {
    const error = this.clubs.error();
    return error instanceof Error ? error.message : 'Could not load clubs.';
  });

  protected goTo(query: PageQuery): void {
    this.page.set(query);
  }
}
