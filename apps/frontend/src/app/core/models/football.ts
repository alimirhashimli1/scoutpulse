/**
 * Wire types for football-svc.
 *
 * These mirror the JSON exactly, field names included. Resist "improving" them
 * into camelCase here — a mapper can do that at the edge of a feature if a view
 * needs it, but the model staying identical to the wire means a mismatch is a
 * compile error rather than a silently undefined field.
 */

/**
 * Money, as an integer count of the currency's smallest unit.
 *
 * 2500 is €25.00. Never a decimal — the API rejects those outright rather than
 * rounding, because 1.5 cents is ambiguous. Do no arithmetic on this in the UI;
 * format it through MoneyPipe.
 *
 * Large transfer fees exceed Number.MAX_SAFE_INTEGER, which is why the API also
 * accepts a quoted form on the way in.
 */
export type MinorUnits = number;

/** An ISO-8601 instant. Dates in this API are dates, but travel as instants. */
export type IsoDate = string;

export type CompetitionType = 'league' | 'domestic_cup' | 'international_cup' | 'super_cup';

export type TransferType =
  'permanent' | 'loan' | 'loan_return' | 'free' | 'youth_promotion' | 'released' | 'retired';

export type Foot = 'left' | 'right' | 'both';

export type SpellRole =
  | 'head_coach'
  | 'assistant_coach'
  | 'interim_coach'
  | 'caretaker'
  | 'director_of_football'
  | 'youth_coach';

export interface League {
  id: string;
  name: string;
  country: string;
  tier?: number;
  competition_type: CompetitionType;
  created_at: IsoDate;
}

export interface Season {
  id: string;
  label: string;
  start_date: IsoDate;
  end_date: IsoDate;
  created_at: IsoDate;
}

export interface Team {
  id: string;
  /** The club's *current* primary competition. History is in TeamSeason. */
  league_id: string | null;
  name: string;
  short_name?: string;
  founded_year?: number;
  stadium?: string;
  city?: string;
  country?: string;
  fan_badge_url?: string;
  created_at: IsoDate;
}

export interface Player {
  id: string;
  /**
   * Derived from transfer history — null means free agent.
   *
   * Sending this to PUT /players/{id} does nothing: the club is maintained by
   * the transfer flow. Moving a player means posting a transfer.
   */
  team_id: string | null;
  name: string;
  first_name?: string;
  last_name?: string;
  date_of_birth?: IsoDate;
  nationality?: string;
  second_nationality?: string;
  height_cm?: number;
  preferred_foot?: Foot;
  agent?: string;
  squad_number?: number;
  position: string;
  contract_start?: IsoDate;
  contract_until?: IsoDate;
  /** Derived: follows the latest recorded valuation. */
  market_value_minor: MinorUnits;
  currency: string;
  created_at: IsoDate;
}

export interface Coach {
  id: string;
  /** Derived from spells — the current appointment. */
  team_id: string | null;
  name: string;
  first_name?: string;
  last_name?: string;
  date_of_birth?: IsoDate;
  nationality?: string;
  created_at: IsoDate;
}

export interface Transfer {
  id: string;
  player_id: string;
  /** null means arriving from nowhere — a youth promotion or an opening record. */
  from_team_id: string | null;
  /** null means leaving the dataset — released or retired. */
  to_team_id: string | null;
  season_id: string | null;
  transfer_date: IsoDate;
  transfer_type: TransferType;
  /** null is *undisclosed*, which is different from a free transfer (0). */
  fee_minor: MinorUnits | null;
  currency: string;
  created_at: IsoDate;
}

export interface MarketValue {
  id: string;
  player_id: string;
  valued_on: IsoDate;
  value_minor: MinorUnits;
  currency: string;
  created_at: IsoDate;
}

export interface CoachSpell {
  id: string;
  coach_id: string;
  team_id: string | null;
  role: SpellRole;
  start_date: IsoDate;
  /** null means the spell is still open — the current appointment. */
  end_date: IsoDate | null;
  created_at: IsoDate;
}

export interface TeamSeason {
  id: string;
  team_id: string;
  season_id: string;
  league_id: string;
  created_at: IsoDate;
}

export interface TeamEditor {
  user_id: string;
  team_id: string;
  granted_by?: string;
  granted_at: string;
}

// --- search ------------------------------------------------------------

export type SearchKind = 'player' | 'team' | 'coach' | 'league';

export interface SearchResult {
  kind: SearchKind;
  id: string;
  name: string;
  /** A short human line: a player's position and nationality, a club's city. */
  subtitle?: string;
  /** Set for players and coaches, so a hit can link straight to the club. */
  team_id?: string;
  rank: number;
}
