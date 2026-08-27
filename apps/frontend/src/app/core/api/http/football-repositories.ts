import { Injectable } from '@angular/core';

import { ApiError } from '../api-error';
import { Page, PageQuery } from '../page';
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
} from '../../models/football';
import {
  CoachReader,
  CoachWriter,
  LeagueReader,
  LeagueWriter,
  PlayerFilter,
  PlayerNoteReader,
  PlayerNoteWriter,
  PlayerReader,
  PlayerWriter,
  SearchReader,
  SeasonReader,
  SeasonWriter,
  TeamEditorReader,
  TeamEditorWriter,
  TeamFilter,
  TeamReader,
  TeamWriter,
  TransferFilter,
  TransferReader,
  TransferWriter,
} from '../contracts';
import { HttpRepository } from './http-repository';

/**
 * HTTP implementations of the football contracts.
 *
 * Reader and writer live in one class per resource — splitting the *interfaces*
 * is what buys the compile-time guarantee (§4); splitting the implementation
 * as well would only mean two files sharing one set of URLs. Consumers still
 * see only the half they were given, because they inject the token, not this.
 */

@Injectable()
export class HttpPlayerRepository
  extends HttpRepository
  implements PlayerReader, PlayerWriter, PlayerNoteReader, PlayerNoteWriter
{
  protected readonly base = this.api.football;

  list(filter?: PlayerFilter): Promise<Page<Player>> {
    return this.getPage<Player>(this.url('players'), filter);
  }

  byId(id: string): Promise<Player> {
    return this.getOne<Player>(this.url('players', id));
  }

  transfers(id: string, query?: PageQuery): Promise<Page<Transfer>> {
    return this.getPage<Transfer>(this.url('players', id, 'transfers'), query);
  }

  marketValues(id: string, query?: PageQuery): Promise<Page<MarketValue>> {
    return this.getPage<MarketValue>(this.url('players', id, 'market-values'), query);
  }

  create(player: Partial<Player>): Promise<Player> {
    return this.post<Player>(this.url('players'), player);
  }

  update(id: string, player: Partial<Player>): Promise<Player> {
    return this.put<Player>(this.url('players', id), { ...player, id });
  }

  remove(id: string): Promise<void> {
    return this.deleteAt(this.url('players', id));
  }

  recordValue(playerId: string, value: Partial<MarketValue>): Promise<MarketValue> {
    return this.post<MarketValue>(this.url('players', playerId, 'market-values'), value);
  }

  removeValue(playerId: string, valueId: string): Promise<void> {
    return this.deleteAt(this.url('players', playerId, 'market-values', valueId));
  }

  // --- member notes ------------------------------------------------------

  notes(playerId: string, query?: PageQuery): Promise<Page<PlayerNote>> {
    return this.getPage<PlayerNote>(this.url('players', playerId, 'notes'), query);
  }

  /**
   * The caller's own note, or null.
   *
   * A 404 here means "you have not written one", which is the ordinary state
   * for most visitors — so it is translated to null rather than thrown. Every
   * other failure still propagates, so a real problem is not swallowed with it.
   */
  async myNote(playerId: string): Promise<PlayerNote | null> {
    try {
      return await this.getOne<PlayerNote>(this.url('players', playerId, 'notes', 'mine'));
    } catch (error) {
      if (error instanceof ApiError && error.code === 'not_found') return null;
      throw error;
    }
  }

  write(playerId: string, body: string): Promise<PlayerNote> {
    return this.post<PlayerNote>(this.url('players', playerId, 'notes'), { body });
  }

  edit(playerId: string, noteId: string, body: string): Promise<PlayerNote> {
    return this.put<PlayerNote>(this.url('players', playerId, 'notes', noteId), { body });
  }

  /**
   * Named `remove` on the contract but `removeNote` here, because the class
   * already has a `remove(id)` for deleting a player. The token is what
   * consumers see, so the collision stays inside this file.
   */
  removeNote(playerId: string, noteId: string): Promise<void> {
    return this.deleteAt(this.url('players', playerId, 'notes', noteId));
  }
}

@Injectable()
export class HttpTeamRepository extends HttpRepository implements TeamReader, TeamWriter {
  protected readonly base = this.api.football;

  list(filter?: TeamFilter): Promise<Page<Team>> {
    return this.getPage<Team>(this.url('teams'), filter);
  }

  byId(id: string): Promise<Team> {
    return this.getOne<Team>(this.url('teams', id));
  }

  /** A club's squad is the player list filtered to it — there is no separate endpoint. */
  squad(id: string, query?: PageQuery): Promise<Page<Player>> {
    return this.getPage<Player>(this.url('players'), { ...query, team_id: id });
  }

  staff(id: string, query?: PageQuery): Promise<Page<CoachSpell>> {
    return this.getPage<CoachSpell>(this.url('teams', id, 'coaches'), query);
  }

  seasons(id: string, query?: PageQuery): Promise<Page<TeamSeason>> {
    return this.getPage<TeamSeason>(this.url('teams', id, 'seasons'), query);
  }

  create(team: Partial<Team>): Promise<Team> {
    return this.post<Team>(this.url('teams'), team);
  }

  update(id: string, team: Partial<Team>): Promise<Team> {
    return this.put<Team>(this.url('teams', id), { ...team, id });
  }

  remove(id: string): Promise<void> {
    return this.deleteAt(this.url('teams', id));
  }

  enterSeason(teamId: string, entry: Partial<TeamSeason>): Promise<TeamSeason> {
    return this.post<TeamSeason>(this.url('teams', teamId, 'seasons'), entry);
  }

  withdrawSeason(teamId: string, entryId: string): Promise<void> {
    return this.deleteAt(this.url('teams', teamId, 'seasons', entryId));
  }
}

@Injectable()
export class HttpLeagueRepository extends HttpRepository implements LeagueReader, LeagueWriter {
  protected readonly base = this.api.football;

  list(query?: PageQuery): Promise<Page<League>> {
    return this.getPage<League>(this.url('leagues'), query);
  }

  byId(id: string): Promise<League> {
    return this.getOne<League>(this.url('leagues', id));
  }

  create(league: Partial<League>): Promise<League> {
    return this.post<League>(this.url('leagues'), league);
  }

  update(id: string, league: Partial<League>): Promise<League> {
    return this.put<League>(this.url('leagues', id), { ...league, id });
  }

  remove(id: string): Promise<void> {
    return this.deleteAt(this.url('leagues', id));
  }
}

@Injectable()
export class HttpSeasonRepository extends HttpRepository implements SeasonReader, SeasonWriter {
  protected readonly base = this.api.football;

  list(query?: PageQuery): Promise<Page<Season>> {
    return this.getPage<Season>(this.url('seasons'), query);
  }

  byId(id: string): Promise<Season> {
    return this.getOne<Season>(this.url('seasons', id));
  }

  current(): Promise<Season> {
    return this.getOne<Season>(this.url('seasons', 'current'));
  }

  teams(seasonId: string, leagueId?: string, query?: PageQuery): Promise<Page<TeamSeason>> {
    return this.getPage<TeamSeason>(this.url('seasons', seasonId, 'teams'), {
      ...query,
      league_id: leagueId,
    });
  }

  create(season: Partial<Season>): Promise<Season> {
    return this.post<Season>(this.url('seasons'), season);
  }

  update(id: string, season: Partial<Season>): Promise<Season> {
    return this.put<Season>(this.url('seasons', id), { ...season, id });
  }

  remove(id: string): Promise<void> {
    return this.deleteAt(this.url('seasons', id));
  }
}

@Injectable()
export class HttpCoachRepository extends HttpRepository implements CoachReader, CoachWriter {
  protected readonly base = this.api.football;

  byId(id: string): Promise<Coach> {
    return this.getOne<Coach>(this.url('coaches', id));
  }

  byTeam(teamId: string): Promise<Coach> {
    return this.getOne<Coach>(this.url('coaches'), { team_id: teamId });
  }

  spells(id: string, query?: PageQuery): Promise<Page<CoachSpell>> {
    return this.getPage<CoachSpell>(this.url('coaches', id, 'spells'), query);
  }

  create(coach: Partial<Coach>): Promise<Coach> {
    return this.post<Coach>(this.url('coaches'), coach);
  }

  update(id: string, coach: Partial<Coach>): Promise<Coach> {
    return this.put<Coach>(this.url('coaches', id), { ...coach, id });
  }

  remove(id: string): Promise<void> {
    return this.deleteAt(this.url('coaches', id));
  }

  recordSpell(coachId: string, spell: Partial<CoachSpell>): Promise<CoachSpell> {
    return this.post<CoachSpell>(this.url('coaches', coachId, 'spells'), spell);
  }

  removeSpell(coachId: string, spellId: string): Promise<void> {
    return this.deleteAt(this.url('coaches', coachId, 'spells', spellId));
  }
}

@Injectable()
export class HttpTransferRepository
  extends HttpRepository
  implements TransferReader, TransferWriter
{
  protected readonly base = this.api.football;

  list(filter?: TransferFilter): Promise<Page<Transfer>> {
    return this.getPage<Transfer>(this.url('transfers'), filter);
  }

  byId(id: string): Promise<Transfer> {
    return this.getOne<Transfer>(this.url('transfers', id));
  }

  record(transfer: Partial<Transfer>): Promise<Transfer> {
    return this.post<Transfer>(this.url('transfers'), transfer);
  }

  update(id: string, transfer: Partial<Transfer>): Promise<Transfer> {
    return this.put<Transfer>(this.url('transfers', id), { ...transfer, id });
  }

  remove(id: string): Promise<void> {
    return this.deleteAt(this.url('transfers', id));
  }
}

/**
 * Editor grants.
 *
 * The odd one out: `teamsFor` reads a wrapper object rather than a bare array,
 * because the endpoint echoes the user id alongside the ids. The wrapper is
 * unwrapped here so no caller has to know the wire shape.
 */
@Injectable()
export class HttpTeamEditorRepository
  extends HttpRepository
  implements TeamEditorReader, TeamEditorWriter
{
  protected readonly base = this.api.football;

  async teamsFor(userId: string): Promise<string[]> {
    const response = await this.getOne<{ user_id: string; team_ids: string[] }>(
      this.url('users', userId, 'teams'),
    );
    return response.team_ids ?? [];
  }

  editors(teamId: string, query?: PageQuery): Promise<Page<TeamEditor>> {
    return this.getPage<TeamEditor>(this.url('teams', teamId, 'editors'), query);
  }

  async grant(teamId: string, userId: string): Promise<void> {
    await this.post<unknown>(this.url('teams', teamId, 'editors'), { user_id: userId });
  }

  revoke(teamId: string, userId: string): Promise<void> {
    return this.deleteAt(this.url('teams', teamId, 'editors', userId));
  }
}

@Injectable()
export class HttpSearchRepository extends HttpRepository implements SearchReader {
  protected readonly base = this.api.football;

  search(query: string, kind?: SearchKind, page?: PageQuery): Promise<Page<SearchResult>> {
    return this.getPage<SearchResult>(this.url('search'), { ...page, q: query, kind });
  }
}
