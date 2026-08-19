import { Injectable, computed, inject, signal } from '@angular/core';

import { League, Season, Team } from '../models/football';
import { LEAGUE_READER, SEASON_READER, TEAM_READER } from './contracts';
import { MAX_PAGE_SIZE, Page, PageQuery } from './page';

/**
 * Resolves club, competition and season ids to names.
 *
 * The API returns ids, not embedded objects — a transfer carries
 * `from_team_id`, not a club, and a competition entry carries three ids and no
 * names at all. That is the right shape for an API (one fact per endpoint, no
 * duplication, no guessing what a caller wants expanded) but it means anything
 * rendering "Süper Lig, 2026/27" has to join.
 *
 * Fetching one record per row would be an N+1: twenty-five rows, fifty
 * requests. Instead each list is loaded once and cached, which works because
 * all three are small — clubs number in the hundreds even for a large dataset,
 * competitions in the dozens, seasons in the tens.
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
  private readonly seasonReader = inject(SEASON_READER);

  private readonly teamsById = signal(new Map<string, Team>());
  private readonly leaguesById = signal(new Map<string, League>());
  private readonly seasonsById = signal(new Map<string, Season>());

  /** In-flight loads, so ten components asking at once produce one request. */
  private teamsLoading: Promise<void> | null = null;
  private leaguesLoading: Promise<void> | null = null;
  private seasonsLoading: Promise<void> | null = null;

  readonly teams = this.teamsById.asReadonly();
  readonly leagues = this.leaguesById.asReadonly();
  readonly seasons = this.seasonsById.asReadonly();

  /**
   * Seasons newest first, for a picker.
   *
   * Sorted by start date rather than by label: labels are free text, and
   * "2026/27" only sorts correctly next to "2025/26" by accident of notation —
   * a label like "Apertura 2026" would not.
   */
  readonly seasonsNewestFirst = computed(() =>
    [...this.seasonsById().values()].sort((a, b) => b.start_date.localeCompare(a.start_date)),
  );

  async loadTeams(): Promise<void> {
    if (this.teamsById().size > 0) return;
    this.teamsLoading ??= this.collect(
      (page) => this.teamReader.list(page),
      this.teamsById,
    ).finally(() => {
      this.teamsLoading = null;
    });
    return this.teamsLoading;
  }

  async loadLeagues(): Promise<void> {
    if (this.leaguesById().size > 0) return;
    this.leaguesLoading ??= this.collect(
      (page) => this.leagueReader.list(page),
      this.leaguesById,
    ).finally(() => {
      this.leaguesLoading = null;
    });
    return this.leaguesLoading;
  }

  async loadSeasons(): Promise<void> {
    if (this.seasonsById().size > 0) return;
    this.seasonsLoading ??= this.collect(
      (page) => this.seasonReader.list(page),
      this.seasonsById,
    ).finally(() => {
      this.seasonsLoading = null;
    });
    return this.seasonsLoading;
  }

  teamName(id: string | null | undefined, fallback = '—'): string {
    if (!id) return fallback;
    return this.teamsById().get(id)?.name ?? fallback;
  }

  leagueName(id: string | null | undefined, fallback = '—'): string {
    if (!id) return fallback;
    return this.leaguesById().get(id)?.name ?? fallback;
  }

  seasonLabel(id: string | null | undefined, fallback = '—'): string {
    if (!id) return fallback;
    return this.seasonsById().get(id)?.label ?? fallback;
  }

  team(id: string | null | undefined): Team | undefined {
    return id ? this.teamsById().get(id) : undefined;
  }

  /**
   * Reads every page of a list into a map keyed by id.
   *
   * Paged rather than one huge request: the API clamps `limit` to 100 whatever
   * is asked for, so a single call would silently truncate — the failure would
   * be a club list that is quietly missing its tail.
   */
  private async collect<T extends { id: string }>(
    fetch: (page: PageQuery) => Promise<Page<T>>,
    into: ReturnType<typeof signal<Map<string, T>>>,
  ): Promise<void> {
    const collected = new Map<string, T>();
    let offset = 0;

    for (;;) {
      const page = await fetch({ limit: MAX_PAGE_SIZE, offset });
      for (const item of page.items) collected.set(item.id, item);
      if (!page.has_more) break;
      offset += page.limit;

      // A safety stop. If has_more were ever wrong this would spin forever,
      // and an infinite request loop is a far worse failure than a short list.
      if (offset > 5_000) break;
    }

    into.set(collected);
  }
}
