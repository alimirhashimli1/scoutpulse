package domain

import "time"

// Date is a calendar date with no time or zone component -- a birth date or a
// transfer date is not an instant. It serializes as "2006-01-02".
type Date = time.Time

// CompetitionType distinguishes the kinds of competition a league row can
// describe. A club contests several of them in one season.
type CompetitionType string

const (
	CompetitionLeague        CompetitionType = "league"
	CompetitionDomesticCup   CompetitionType = "domestic_cup"
	CompetitionInternational CompetitionType = "international_cup"
	CompetitionSuperCup      CompetitionType = "super_cup"
)

// ValidCompetitionType reports whether t is a recognised competition type.
func ValidCompetitionType(t CompetitionType) bool {
	switch t {
	case CompetitionLeague, CompetitionDomesticCup, CompetitionInternational, CompetitionSuperCup:
		return true
	default:
		return false
	}
}

// League represents a competition: a domestic league, a cup, or a continental
// tournament.
type League struct {
	ID              string          `db:"id" json:"id"`
	Name            string          `db:"name" json:"name"`
	Country         string          `db:"country" json:"country"`
	Tier            *int            `db:"tier" json:"tier,omitempty"`
	CompetitionType CompetitionType `db:"competition_type" json:"competition_type"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
}

// Season is the annual window competitions are played within. It is what makes
// the rest of the model temporal: a club's league, a squad, and a transfer are
// all statements about a particular season.
type Season struct {
	ID        string    `db:"id" json:"id"`
	Label     string    `db:"label" json:"label"` // e.g. "2025/26"
	StartDate Date      `db:"start_date" json:"start_date"`
	EndDate   Date      `db:"end_date" json:"end_date"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// Team represents a football club.
type Team struct {
	ID   string `db:"id" json:"id"`
	Name string `db:"name" json:"name"`
	// LeagueID is the club's current primary competition. Historical
	// participation lives in team_seasons.
	LeagueID    *string   `db:"league_id" json:"league_id"`
	ShortName   *string   `db:"short_name" json:"short_name,omitempty"`
	FoundedYear *int      `db:"founded_year" json:"founded_year,omitempty"`
	Stadium     *string   `db:"stadium" json:"stadium,omitempty"`
	City        *string   `db:"city" json:"city,omitempty"`
	Country     *string   `db:"country" json:"country,omitempty"`
	FanBadgeURL *string   `db:"fan_badge_url" json:"fan_badge_url,omitempty"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

// TeamSeason records that a club contested a competition in a season.
type TeamSeason struct {
	ID        string    `db:"id" json:"id"`
	TeamID    string    `db:"team_id" json:"team_id"`
	SeasonID  string    `db:"season_id" json:"season_id"`
	LeagueID  string    `db:"league_id" json:"league_id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// SpellRole is the capacity in which a coach served a club.
type SpellRole string

const (
	SpellHeadCoach      SpellRole = "head_coach"
	SpellAssistantCoach SpellRole = "assistant_coach"
	SpellInterimCoach   SpellRole = "interim_coach"
	SpellCaretaker      SpellRole = "caretaker"
	SpellDirector       SpellRole = "director_of_football"
	SpellYouthCoach     SpellRole = "youth_coach"
)

// ValidSpellRole reports whether r is a recognised role. The empty string is
// allowed and defaults to head_coach at the service layer.
func ValidSpellRole(r SpellRole) bool {
	switch r {
	case "", SpellHeadCoach, SpellAssistantCoach, SpellInterimCoach,
		SpellCaretaker, SpellDirector, SpellYouthCoach:
		return true
	default:
		return false
	}
}

// Foot is a player's preferred foot.
type Foot string

const (
	FootLeft  Foot = "left"
	FootRight Foot = "right"
	FootBoth  Foot = "both"
)

// ValidFoot reports whether f is a recognised value. The empty string is
// allowed and means "unknown".
func ValidFoot(f Foot) bool {
	switch f {
	case "", FootLeft, FootRight, FootBoth:
		return true
	default:
		return false
	}
}

// Player represents a footballer.
//
// TeamID and MarketValueMinor are derived state: the club is maintained by the
// transfer flow (transfers is the source of truth) and the value by the
// valuation flow (player_market_values is the source of truth). They are stored
// here so the common reads stay a single-table lookup.
type Player struct {
	ID     string  `db:"id" json:"id"`
	TeamID *string `db:"team_id" json:"team_id"` // nil = free agent
	Name   string  `db:"name" json:"name"`

	FirstName   *string `db:"first_name" json:"first_name,omitempty"`
	LastName    *string `db:"last_name" json:"last_name,omitempty"`
	DateOfBirth *Date   `db:"date_of_birth" json:"date_of_birth,omitempty"`

	// Nationalities is ordered, and the first entry is the primary one.
	//
	// This replaced a nationality/second_nationality pair, which could only
	// ever express two and left "second" undefined for someone holding three.
	Nationalities StringList `db:"nationalities" json:"nationalities"`

	HeightCM      *int    `db:"height_cm" json:"height_cm,omitempty"`
	PreferredFoot *Foot   `db:"preferred_foot" json:"preferred_foot,omitempty"`
	Agent         *string `db:"agent" json:"agent,omitempty"`
	SquadNumber   *int    `db:"squad_number" json:"squad_number,omitempty"`

	// Position is the one a player is listed as. SecondaryPositions are the
	// others they can fill — a different fact, and deliberately separate so
	// squad lists and filters keep a single answer for "what is he?".
	Position           string     `db:"position" json:"position"`
	SecondaryPositions StringList `db:"secondary_positions" json:"secondary_positions"`

	ContractStart *Date  `db:"contract_start" json:"contract_start,omitempty"`
	ContractUntil *Date  `db:"contract_until" json:"contract_until,omitempty"`
	MarketValue   Minor  `db:"market_value_minor" json:"market_value_minor"`
	Currency      string `db:"currency" json:"currency"`

	// Scouting percentages, 0-100, entered by hand.
	//
	// **Not computed from matches.** There is no match data in this system, so
	// these are a scout's recorded averages rather than anything derived. Nil
	// means not recorded, which is a different fact from zero.
	DuelsWonPct       *float64 `db:"duels_won_pct" json:"duels_won_pct,omitempty"`
	PassCompletionPct *float64 `db:"pass_completion_pct" json:"pass_completion_pct,omitempty"`
	ShotsOnTargetPct  *float64 `db:"shots_on_target_pct" json:"shots_on_target_pct,omitempty"`
	AerialDuelsWonPct *float64 `db:"aerial_duels_won_pct" json:"aerial_duels_won_pct,omitempty"`

	// The two halves of the scouting assessment. Lists rather than prose, so
	// they render as bullets and stay comparable between players.
	Strengths  StringList `db:"strengths" json:"strengths"`
	Weaknesses StringList `db:"weaknesses" json:"weaknesses"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// PlayerNote is one member's note on a player.
//
// At most one per person per player, enforced by a unique constraint rather
// than by a check in the handler: two simultaneous posts would both pass a
// "does one already exist?" test and both insert.
type PlayerNote struct {
	ID       string `db:"id" json:"id"`
	PlayerID string `db:"player_id" json:"player_id"`
	AuthorID string `db:"author_id" json:"author_id"`
	// AuthorName is captured when the note is written, because this service
	// cannot resolve a user id to a name -- accounts live in identity-svc. It
	// also means the note keeps the name it was written under after the
	// account is gone.
	AuthorName string    `db:"author_name" json:"author_name"`
	Body       string    `db:"body" json:"body"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time `db:"updated_at" json:"updated_at"`
}

// Age returns the player's age in completed years as of now, or nil when the
// date of birth is unknown.
func (p Player) Age() *int {
	if p.DateOfBirth == nil {
		return nil
	}

	now := time.Now().UTC()
	years := now.Year() - p.DateOfBirth.Year()
	// Subtract a year if the birthday has not occurred yet this year.
	if now.YearDay() < p.DateOfBirth.YearDay() {
		years--
	}
	if years < 0 {
		return nil
	}
	return &years
}

// TransferType classifies how a player moved.
type TransferType string

const (
	TransferPermanent      TransferType = "permanent"
	TransferLoan           TransferType = "loan"
	TransferLoanReturn     TransferType = "loan_return"
	TransferFree           TransferType = "free"
	TransferYouthPromotion TransferType = "youth_promotion"
	TransferReleased       TransferType = "released"
	TransferRetired        TransferType = "retired"
)

// ValidTransferType reports whether t is a recognised transfer type.
func ValidTransferType(t TransferType) bool {
	switch t {
	case TransferPermanent, TransferLoan, TransferLoanReturn,
		TransferFree, TransferYouthPromotion, TransferReleased, TransferRetired:
		return true
	default:
		return false
	}
}

// Transfer is one move in a player's career, and the source of truth for where
// a player has played.
type Transfer struct {
	ID       string  `db:"id" json:"id"`
	PlayerID string  `db:"player_id" json:"player_id"`
	FromTeam *string `db:"from_team_id" json:"from_team_id"` // nil = joined from outside the dataset
	ToTeam   *string `db:"to_team_id" json:"to_team_id"`     // nil = released or retired
	SeasonID *string `db:"season_id" json:"season_id,omitempty"`

	TransferDate Date         `db:"transfer_date" json:"transfer_date"`
	Type         TransferType `db:"transfer_type" json:"transfer_type"`
	// Fee is nil for an undisclosed fee, which is distinct from a free
	// transfer (type "free", fee 0).
	Fee       *Minor    `db:"fee_minor" json:"fee_minor"`
	Currency  string    `db:"currency" json:"currency"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// MarketValue is one point in a player's valuation history.
type MarketValue struct {
	ID        string    `db:"id" json:"id"`
	PlayerID  string    `db:"player_id" json:"player_id"`
	ValuedOn  Date      `db:"valued_on" json:"valued_on"`
	Value     Minor     `db:"value_minor" json:"value_minor"`
	Currency  string    `db:"currency" json:"currency"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// Coach represents a manager or member of coaching staff. TeamID is the
// current appointment; the full record is in coach_spells.
type Coach struct {
	ID            string     `db:"id" json:"id"`
	TeamID        *string    `db:"team_id" json:"team_id"`
	Name          string     `db:"name" json:"name"`
	FirstName     *string    `db:"first_name" json:"first_name,omitempty"`
	LastName      *string    `db:"last_name" json:"last_name,omitempty"`
	DateOfBirth   *Date      `db:"date_of_birth" json:"date_of_birth,omitempty"`
	Nationality   *string    `db:"nationality" json:"nationality,omitempty"`
	ContractUntil *time.Time `db:"contract_until" json:"contract_until,omitempty"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
}

// CoachSpell is one appointment in a coach's career.
type CoachSpell struct {
	ID        string    `db:"id" json:"id"`
	CoachID   string    `db:"coach_id" json:"coach_id"`
	TeamID    *string   `db:"team_id" json:"team_id"`
	Role      SpellRole `db:"role" json:"role"`
	StartDate Date      `db:"start_date" json:"start_date"`
	EndDate   *Date     `db:"end_date" json:"end_date,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
