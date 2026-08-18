import { Injectable, inject, signal } from '@angular/core';

import { League, Team } from '../models/football';
import { LEAGUE_READER, TEAM_READER } from './contracts';
import { MAX_PAGE_SIZE } from './page';

/**
 * Resolves club and competition ids to names.
 *
 * The API returns ids, not embedded objects — a transfer carries
 * `from_team_id`, not a club. That is the right shape for an API (one fact per
 * endpoint, no duplication, no guessing what a caller wants expanded) but it
 * means a transfer feed showing "Selling FC → Buying FC" has to join.
 *
 * Fetching a club per transfer would be an N+1: twenty-five rows, fifty
 * requests. Instead the club and competition lists are loaded once and cached,
 * which works because both are small — clubs number in the hundreds even for a
 * large dataset, and competitions in the dozens.
 *
 * **This is the piece that will not scale**, and it is worth being honest about
 * where the limit is. Past a few thousand clubs, this should become either an
 * `?expand=` parameter on the API or a batch `GET /teams?ids=` endpoint. Until
 * then, one request beats fifty.
 */
@Injectable({ providedIn: 'root' })
export class LookupStore {
  private readonly teamReader = inject(TEAM_READER);
  private readonly leagueReader = inject(LEAGUE_READER);

  private readonly teamsById = signal(new Map<string, Team>());
  private readonly leaguesById = signal(new Map<string, League>());

  /** In-flight loads, so ten components asking at once produce one request. */
  private teamsLoading: Promise<void> | null = null;
  private leaguesLoading: Promise<void> | null = null;

  readonly teams = this.teamsById.asReadonly();
  readonly leagues = this.leaguesById.asReadonly();

  async loadTeams(): Promise<void> {
    if (this.teamsById().size > 0) return;
    this.teamsLoading ??= this.fetchAllTeams().finally(() => {
      this.teamsLoading = null;
    });
    return this.teamsLoading;
  }

  async loadLeagues(): Promise<void> {
    if (this.leaguesById().size > 0) return;
    this.leaguesLoading ??= this.fetchAllLeagues().finally(() => {
      this.leaguesLoading = null;
    });
    return this.leaguesLoading;
  }

  teamName(id: string | null | undefined, fallback = '—'): string {
    if (!id) return fallback;
    return this.teamsById().get(id)?.name ?? fallback;
  }

  leagueName(id: string | null | undefined, fallback = '—'): string {
    if (!id) return fallback;
    return this.leaguesById().get(id)?.name ?? fallback;
  }

  team(id: string | null | undefined): Team | undefined {
    return id ? this.teamsById().get(id) : undefined;
  }

  private async fetchAllTeams(): Promise<void> {
    const collected = new Map<string, Team>();
    let offset = 0;

    // Paged rather than one huge request: the API clamps limit to 100 whatever
    // is asked for, so a single call would silently truncate.
    for (;;) {
      const page = await this.teamReader.list({ limit: MAX_PAGE_SIZE, offset });
      for (const team of page.items) collected.set(team.id, team);
      if (!page.has_more) break;
      offset += page.limit;

      // A safety stop. If has_more were ever wrong this would spin forever,
      // and an infinite request loop is a far worse failure than a short list.
      if (offset > 5_000) break;
    }

    this.teamsById.set(collected);
  }

  private async fetchAllLeagues(): Promise<void> {
    const collected = new Map<string, League>();
    let offset = 0;

    for (;;) {
      const page = await this.leagueReader.list({ limit: MAX_PAGE_SIZE, offset });
      for (const league of page.items) collected.set(league.id, league);
      if (!page.has_more) break;
      offset += page.limit;
      if (offset > 5_000) break;
    }

    this.leaguesById.set(collected);
  }
}
