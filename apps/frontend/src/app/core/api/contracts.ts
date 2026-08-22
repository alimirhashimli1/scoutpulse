import { InjectionToken } from '@angular/core';

import { Page, PageQuery } from './page';
import {
  Coach,
  CoachSpell,
  League,
  MarketValue,
  Player,
  PlayerNote,
  SearchKind,
  SearchResult,
  Season,
  Team,
  TeamEditor,
  TeamSeason,
  Transfer,
} from '../models/football';

/**
 * Repository contracts.
 *
 * Two deliberate choices here, both from docs/FRONTEND.md §4:
 *
 * **Reads and writes are separate interfaces.** A public page injects only the
 * reader, and therefore *cannot* call a write method — the compiler enforces
 * the read-only page rather than a reviewer noticing. It also means the far
 * larger read surface is testable without stubbing writes that will never be
 * called.
 *
 * **Consumers depend on tokens, never on classes.** Swapping the HTTP
 * implementation for an in-memory one in a component test is a provider line,
 * not a change to the component.
 */

// --- players -----------------------------------------------------------

export interface PlayerFilter extends PageQuery {
  team_id?: string;
  position?: string;
  nationality?: string;
  free_agent?: boolean;
  min_value_minor?: number;
  max_value_minor?: number;
  /**
   * Resolve a known set of players in one request.
   *
   * Serialised by `String(value)`, so an array becomes `a,b,c` — which is the
   * form the API parses. Capped server-side at 100 ids.
   */
  ids?: string[];
}

export interface PlayerReader {
  list(filter?: PlayerFilter): Promise<Page<Player>>;
  byId(id: string): Promise<Player>;
  transfers(id: string, query?: PageQuery): Promise<Page<Transfer>>;
  marketValues(id: string, query?: PageQuery): Promise<Page<MarketValue>>;
}

export interface PlayerWriter {
  create(player: Partial<Player>): Promise<Player>;
  /**
   * Descriptive fields only. `team_id` is ignored by the API — a player's club
   * is derived from transfer history, so moving one means recording a
   * transfer. See §2.4.
   */
  update(id: string, player: Partial<Player>): Promise<Player>;
  remove(id: string): Promise<void>;
  recordValue(playerId: string, value: Partial<MarketValue>): Promise<MarketValue>;
  removeValue(playerId: string, valueId: string): Promise<void>;
}

export const PLAYER_READER = new InjectionToken<PlayerReader>('PlayerReader');
export const PLAYER_WRITER = new InjectionToken<PlayerWriter>('PlayerWriter');

// --- member notes on a player ------------------------------------------

export interface PlayerNoteReader {
  /** A player's notes, newest first. Public — the commentary is the product. */
  notes(playerId: string, query?: PageQuery): Promise<Page<PlayerNote>>;
  /**
   * The signed-in member's own note, or null when they have not written one.
   *
   * Null rather than a rejected promise: "you have not commented yet" is the
   * ordinary case, and the API says it with a 404 that is not an error here.
   */
  myNote(playerId: string): Promise<PlayerNote | null>;
}

export interface PlayerNoteWriter {
  /** Refused with a conflict if this member already has a note on the player. */
  write(playerId: string, body: string): Promise<PlayerNote>;
  edit(playerId: string, noteId: string, body: string): Promise<PlayerNote>;
  /**
   * Named removeNote rather than remove: the HTTP class implements both this
   * and PlayerWriter, whose remove(id) deletes a player. Two methods called
   * remove with different meanings on one object is a trap.
   */
  removeNote(playerId: string, noteId: string): Promise<void>;
}

export const PLAYER_NOTE_READER = new InjectionToken<PlayerNoteReader>('PlayerNoteReader');
export const PLAYER_NOTE_WRITER = new InjectionToken<PlayerNoteWriter>('PlayerNoteWriter');

// --- clubs -------------------------------------------------------------

export interface TeamFilter extends PageQuery {
  /** Optional. Omit it to list every club. */
  league_id?: string;
}

export interface TeamReader {
  list(filter?: TeamFilter): Promise<Page<Team>>;
  byId(id: string): Promise<Team>;
  squad(id: string, query?: PageQuery): Promise<Page<Player>>;
  staff(id: string, query?: PageQuery): Promise<Page<CoachSpell>>;
  seasons(id: string, query?: PageQuery): Promise<Page<TeamSeason>>;
}

export interface TeamWriter {
  create(team: Partial<Team>): Promise<Team>;
  /** `league_id` is preserved unless the caller is an administrator. */
  update(id: string, team: Partial<Team>): Promise<Team>;
  remove(id: string): Promise<void>;
  enterSeason(teamId: string, entry: Partial<TeamSeason>): Promise<TeamSeason>;
  withdrawSeason(teamId: string, entryId: string): Promise<void>;
}

export const TEAM_READER = new InjectionToken<TeamReader>('TeamReader');
export const TEAM_WRITER = new InjectionToken<TeamWriter>('TeamWriter');

// --- competitions ------------------------------------------------------

export interface LeagueReader {
  list(query?: PageQuery): Promise<Page<League>>;
  byId(id: string): Promise<League>;
}

export interface LeagueWriter {
  create(league: Partial<League>): Promise<League>;
  update(id: string, league: Partial<League>): Promise<League>;
  remove(id: string): Promise<void>;
}

export const LEAGUE_READER = new InjectionToken<LeagueReader>('LeagueReader');
export const LEAGUE_WRITER = new InjectionToken<LeagueWriter>('LeagueWriter');

// --- seasons -----------------------------------------------------------

export interface SeasonReader {
  list(query?: PageQuery): Promise<Page<Season>>;
  byId(id: string): Promise<Season>;
  /** The season containing today. Rejects with `not_found` when none does. */
  current(): Promise<Season>;
  teams(seasonId: string, leagueId?: string, query?: PageQuery): Promise<Page<TeamSeason>>;
}

export interface SeasonWriter {
  create(season: Partial<Season>): Promise<Season>;
  update(id: string, season: Partial<Season>): Promise<Season>;
  remove(id: string): Promise<void>;
}

export const SEASON_READER = new InjectionToken<SeasonReader>('SeasonReader');
export const SEASON_WRITER = new InjectionToken<SeasonWriter>('SeasonWriter');

// --- coaches -----------------------------------------------------------

export interface CoachReader {
  byId(id: string): Promise<Coach>;
  byTeam(teamId: string): Promise<Coach>;
  spells(id: string, query?: PageQuery): Promise<Page<CoachSpell>>;
}

export interface CoachWriter {
  create(coach: Partial<Coach>): Promise<Coach>;
  update(id: string, coach: Partial<Coach>): Promise<Coach>;
  remove(id: string): Promise<void>;
  recordSpell(coachId: string, spell: Partial<CoachSpell>): Promise<CoachSpell>;
  removeSpell(coachId: string, spellId: string): Promise<void>;
}

export const COACH_READER = new InjectionToken<CoachReader>('CoachReader');
export const COACH_WRITER = new InjectionToken<CoachWriter>('CoachWriter');

// --- transfers ---------------------------------------------------------

export interface TransferFilter extends PageQuery {
  player_id?: string;
  /** Matches either side of a move. */
  team_id?: string;
  season_id?: string;
  type?: string;
  /** Undisclosed fees are excluded when this is set — null cannot compare. */
  min_fee_minor?: number;
}

export interface TransferReader {
  list(filter?: TransferFilter): Promise<Page<Transfer>>;
  byId(id: string): Promise<Transfer>;
}

export interface TransferWriter {
  record(transfer: Partial<Transfer>): Promise<Transfer>;
  /**
   * Corrects type, fee, currency and season only. The player, both clubs and
   * the date come from the stored row whatever is sent.
   */
  update(id: string, transfer: Partial<Transfer>): Promise<Transfer>;
  remove(id: string): Promise<void>;
}

export const TRANSFER_READER = new InjectionToken<TransferReader>('TransferReader');
export const TRANSFER_WRITER = new InjectionToken<TransferWriter>('TransferWriter');

// --- editor grants -----------------------------------------------------

export interface TeamEditorReader {
  /**
   * The clubs a user may edit.
   *
   * A caller may always read their own grants; only an administrator may read
   * anyone else's. This is what lets the UI offer exactly the clubs someone
   * can actually write to, instead of showing every club and letting the API
   * answer with a 403.
   */
  teamsFor(userId: string): Promise<string[]>;
  /** Who may edit a club. Administrators only — this is access-control data. */
  editors(teamId: string, query?: PageQuery): Promise<Page<TeamEditor>>;
}

export interface TeamEditorWriter {
  grant(teamId: string, userId: string): Promise<void>;
  revoke(teamId: string, userId: string): Promise<void>;
}

export const TEAM_EDITOR_READER = new InjectionToken<TeamEditorReader>('TeamEditorReader');
export const TEAM_EDITOR_WRITER = new InjectionToken<TeamEditorWriter>('TeamEditorWriter');

// --- search ------------------------------------------------------------

export interface SearchReader {
  /** Ranked hits across players, clubs, coaches and competitions. */
  search(query: string, kind?: SearchKind, page?: PageQuery): Promise<Page<SearchResult>>;
}

export const SEARCH_READER = new InjectionToken<SearchReader>('SearchReader');
