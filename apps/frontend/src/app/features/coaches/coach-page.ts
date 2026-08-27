import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  resource,
} from '@angular/core';
import { DatePipe } from '@angular/common';
import { RouterLink } from '@angular/router';

import { ApiError } from '../../core/api/api-error';
import { COACH_READER } from '../../core/api/contracts';
import { LookupStore } from '../../core/api/lookup-store';
import { Permissions } from '../../core/auth/permissions';
import { Seo } from '../../core/seo/seo';
import { Actions } from '../../shared/ui/actions';
import { ErrorState, Loading } from '../../shared/ui/states';
import { ssrResource } from '../../core/api/ssr-resource';

/**
 * A coach and their career.
 *
 * This page was impossible until `GET /coaches/{id}` existed — a coach could
 * only be reached through a club, so there was nothing to load a profile from.
 */
@Component({
  selector: 'app-coach-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, DatePipe, Actions, Loading, ErrorState],
  template: `
    @if (coach.isLoading() && !coach.value()) {
      <main class="page"><app-loading message="Loading coach…" /></main>
    } @else if (coach.error() && !coach.value()) {
      <main class="page"><app-error-state [message]="errorMessage()" /></main>
    } @else if (coach.value(); as c) {
      <main class="page">
        <header class="head">
          <p class="eyebrow">Coach</p>
          <h1>{{ c.name }}</h1>

          <dl class="facts">
            <div>
              <dt>Club</dt>
              <dd>
                @if (c.team_id) {
                  <a [routerLink]="['/clubs', c.team_id]">{{ clubName(c.team_id) }}</a>
                } @else {
                  <span class="muted">Unattached</span>
                }
              </dd>
            </div>
            @if (c.nationality) {
              <div>
                <dt>Nationality</dt>
                <dd>{{ c.nationality }}</dd>
              </div>
            }
            @if (c.date_of_birth) {
              <div>
                <dt>Born</dt>
                <dd>{{ c.date_of_birth | date: 'd MMM y' }}</dd>
              </div>
            }
          </dl>

          <!--
            An unattached coach has no club to hold a grant over, so only an
            administrator sees these. That is the same rule as an unattached
            player: no club means no grant can cover it.
          -->
          @if (permissions.canEditTeam(c.team_id)) {
            <app-actions>
              <a class="btn" [routerLink]="['/coaches', c.id, 'edit']">Edit details</a>
              <a class="btn primary" [routerLink]="['/coaches', c.id, 'spells', 'new']">
                Record an appointment
              </a>
            </app-actions>
          }
        </header>

        <section>
          <h4>Career</h4>
          @if (spells.value()?.items?.length) {
            <ul class="spells">
              @for (spell of spells.value()!.items; track spell.id) {
                <li>
                  <span class="club">
                    @if (spell.team_id) {
                      <a [routerLink]="['/clubs', spell.team_id]">{{ clubName(spell.team_id) }}</a>
                    } @else {
                      <!-- The club was deleted; ON DELETE SET NULL kept the spell. -->
                      <span class="muted">Former club</span>
                    }
                  </span>
                  <span class="role">{{ spell.role.replace('_', ' ') }}</span>
                  <span class="dates tabular">
                    {{ spell.start_date | date: 'MMM y' }} –
                    {{ spell.end_date ? (spell.end_date | date: 'MMM y') : 'present' }}
                  </span>
                </li>
              }
            </ul>
          } @else {
            <p class="muted">No spells recorded.</p>
          }
        </section>
      </main>
    }
  `,
  styles: `
    .head {
      border-bottom: 1px solid var(--line);
      padding-block: var(--space-6) var(--space-5);
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
      font-size: var(--text-3xl);
      margin-bottom: var(--space-5);
    }
    .facts {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr));
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
    h4 {
      margin-bottom: var(--space-3);
    }
    .spells {
      list-style: none;
      margin: 0;
      padding: 0;
    }
    .spells li {
      display: flex;
      gap: var(--space-4);
      align-items: baseline;
      padding: var(--space-3) 0;
      border-bottom: 1px solid var(--line-soft);
    }
    .club {
      font-weight: 600;
    }
    .role {
      font-family: var(--font-mono);
      font-size: 11px;
      color: var(--ink-soft);
    }
    .dates {
      color: var(--muted);
      font-size: var(--text-sm);
      margin-left: auto;
    }
    .muted {
      color: var(--muted);
    }
    app-actions {
      display: block;
      margin-top: var(--space-5);
    }
  `,
})
export class CoachPage {
  private readonly reader = inject(COACH_READER);
  private readonly lookup = inject(LookupStore);
  private readonly seo = inject(Seo);
  protected readonly permissions = inject(Permissions);

  readonly id = input.required<string>();

  protected readonly coach = ssrResource('coach-page.coach', {
    params: () => ({ id: this.id() }),
    loader: async ({ params }) => {
      await this.lookup.loadTeams();
      return this.reader.byId(params.id);
    },
  });

  protected readonly spells = ssrResource('coach-page.spells', {
    params: () => ({ id: this.id() }),
    loader: ({ params }) => this.reader.spells(params.id, { limit: 50 }),
  });

  constructor() {
    effect(() => {
      const c = this.coach.value();
      if (!c) return;

      const club = c.team_id ? this.lookup.teamName(c.team_id, 'a club') : 'unattached';
      this.seo.describe({
        title: c.name,
        description: `${c.name} — coaching career and appointments. Currently ${club}.`,
        path: `/coaches/${c.id}`,
        type: 'profile',
      });

      this.seo.structuredData({
        '@context': 'https://schema.org',
        '@type': 'Person',
        name: c.name,
        givenName: c.first_name,
        familyName: c.last_name,
        birthDate: c.date_of_birth?.slice(0, 10),
        nationality: c.nationality,
        jobTitle: 'Football coach',
        worksFor: c.team_id
          ? { '@type': 'SportsTeam', name: this.lookup.teamName(c.team_id, 'Club') }
          : undefined,
      });
    });
  }

  protected readonly errorMessage = computed(() => {
    const error = this.coach.error();
    if (error instanceof ApiError && error.code === 'not_found') return 'No coach with that id.';
    return error instanceof Error ? error.message : 'Could not load the coach.';
  });

  protected clubName(id: string | null): string {
    return this.lookup.teamName(id, '—');
  }
}
