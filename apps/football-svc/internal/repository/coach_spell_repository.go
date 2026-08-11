package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/libs/platform/apperr"
)

type CoachSpellRepository interface {
	ListByCoach(ctx context.Context, coachID string, page domain.Page) ([]domain.CoachSpell, error)
	ListByTeam(ctx context.Context, teamID string, page domain.Page) ([]domain.CoachSpell, error)
	// Record opens a spell and updates the coach's current club atomically,
	// closing any spell still open at that club.
	Record(ctx context.Context, spell *domain.CoachSpell) error
	Delete(ctx context.Context, id string) error
}

type postgresCoachSpellRepository struct {
	db *sqlx.DB
}

func NewPostgresCoachSpellRepository(db *sqlx.DB) CoachSpellRepository {
	return &postgresCoachSpellRepository{db: db}
}

const coachSpellColumns = `id, coach_id, team_id, role, start_date, end_date, created_at`

func (r *postgresCoachSpellRepository) ListByCoach(ctx context.Context, coachID string, page domain.Page) ([]domain.CoachSpell, error) {
	var spells []domain.CoachSpell
	query := `SELECT ` + coachSpellColumns + ` FROM coach_spells
	          WHERE coach_id = $1 ORDER BY start_date DESC, id LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &spells, query, coachID, page.FetchLimit(), page.Offset); err != nil {
		return nil, translate("coach spell", err)
	}
	return spells, nil
}

func (r *postgresCoachSpellRepository) ListByTeam(ctx context.Context, teamID string, page domain.Page) ([]domain.CoachSpell, error) {
	var spells []domain.CoachSpell
	query := `SELECT ` + coachSpellColumns + ` FROM coach_spells
	          WHERE team_id = $1 ORDER BY start_date DESC, id LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &spells, query, teamID, page.FetchLimit(), page.Offset); err != nil {
		return nil, translate("coach spell", err)
	}
	return spells, nil
}

func (r *postgresCoachSpellRepository) Record(ctx context.Context, s *domain.CoachSpell) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translate("coach spell", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Is this the coach's latest spell, or is it being backfilled behind one
	// already recorded? Everything below depends on the answer, so it is
	// established once, inside the transaction.
	var laterExists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM coach_spells WHERE coach_id = $1 AND start_date > $2)`,
		s.CoachID, s.StartDate).Scan(&laterExists); err != nil {
		return translate("coach spell", err)
	}

	// Closing an open spell at the new start date only makes sense when the
	// new spell actually follows it. Backdating behind an open spell would set
	// an end_date earlier than that spell's own start and trip the
	// coach_spells_dates_ordered constraint, surfacing as an opaque database
	// error rather than a clear rejection.
	if !laterExists {
		// A coach cannot hold two open spells at once; the previous
		// appointment ends the day the new one begins.
		closePrevious := `UPDATE coach_spells SET end_date = $1
		                   WHERE coach_id = $2 AND end_date IS NULL AND start_date <= $1`
		if _, err := tx.ExecContext(ctx, closePrevious, s.StartDate, s.CoachID); err != nil {
			return translate("coach spell", err)
		}
	} else if s.EndDate == nil {
		return apperr.Invalid("this coach has a later spell on record; a backdated spell must have an end_date")
	}

	insert := `INSERT INTO coach_spells (coach_id, team_id, role, start_date, end_date)
	           VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`
	if err := tx.QueryRowContext(ctx, insert, s.CoachID, s.TeamID, s.Role, s.StartDate, s.EndDate).
		Scan(&s.ID, &s.CreatedAt); err != nil {
		return translate("coach spell", err)
	}

	// Only an open spell that is also the coach's most recent one describes the
	// current appointment. Without the laterExists guard, filing a 2015 spell
	// would silently make that club the coach's current employer -- the same
	// mistake the valuation flow guards against when an older figure is
	// backfilled.
	if s.EndDate == nil && !laterExists {
		res, err := tx.ExecContext(ctx, `UPDATE coaches SET team_id = $1 WHERE id = $2`, s.TeamID, s.CoachID)
		if err := affected("coach", res, err); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return translate("coach spell", err)
	}
	return nil
}

func (r *postgresCoachSpellRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM coach_spells WHERE id = $1`, id)
	return affected("coach spell", res, err)
}
