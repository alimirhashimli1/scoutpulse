package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
)

// TeamEditor records that a user may edit a club.
type TeamEditor struct {
	UserID    string  `db:"user_id" json:"user_id"`
	TeamID    string  `db:"team_id" json:"team_id"`
	GrantedBy *string `db:"granted_by" json:"granted_by,omitempty"`
	GrantedAt string  `db:"granted_at" json:"granted_at"`
}

type TeamEditorRepository interface {
	// Manages reports whether the user may edit the club. This is the hot
	// path: it runs on every editor write.
	Manages(ctx context.Context, userID, teamID string) (bool, error)
	ListTeams(ctx context.Context, userID string) ([]string, error)
	ListEditors(ctx context.Context, teamID string, page domain.Page) ([]TeamEditor, error)
	Grant(ctx context.Context, userID, teamID string, grantedBy *string) error
	Revoke(ctx context.Context, userID, teamID string) error
	// RevokeAllForUser drops every grant a user holds and reports how many
	// went. Used when the identity service says the account was deleted:
	// there is no foreign key to do it, because users live in another
	// service's database.
	RevokeAllForUser(ctx context.Context, userID string) (int64, error)
}

type postgresTeamEditorRepository struct {
	db *sqlx.DB
}

func NewPostgresTeamEditorRepository(db *sqlx.DB) TeamEditorRepository {
	return &postgresTeamEditorRepository{db: db}
}

func (r *postgresTeamEditorRepository) Manages(ctx context.Context, userID, teamID string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS (SELECT 1 FROM team_editors WHERE user_id = $1 AND team_id = $2)`
	if err := r.db.GetContext(ctx, &exists, query, userID, teamID); err != nil {
		return false, translate("team editor", err)
	}
	return exists, nil
}

func (r *postgresTeamEditorRepository) ListTeams(ctx context.Context, userID string) ([]string, error) {
	var ids []string
	query := `SELECT team_id FROM team_editors WHERE user_id = $1 ORDER BY team_id`
	if err := r.db.SelectContext(ctx, &ids, query, userID); err != nil {
		return nil, translate("team editor", err)
	}
	return ids, nil
}

func (r *postgresTeamEditorRepository) ListEditors(ctx context.Context, teamID string, page domain.Page) ([]TeamEditor, error) {
	var editors []TeamEditor
	query := `SELECT user_id, team_id, granted_by, granted_at FROM team_editors
	          WHERE team_id = $1 ORDER BY granted_at DESC, user_id LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &editors, query, teamID, page.FetchLimit(), page.Offset); err != nil {
		return nil, translate("team editor", err)
	}
	return editors, nil
}

func (r *postgresTeamEditorRepository) Grant(ctx context.Context, userID, teamID string, grantedBy *string) error {
	// Re-granting an existing permission is a no-op rather than a conflict.
	query := `INSERT INTO team_editors (user_id, team_id, granted_by) VALUES ($1, $2, $3)
	          ON CONFLICT (user_id, team_id) DO NOTHING`
	_, err := r.db.ExecContext(ctx, query, userID, teamID, grantedBy)
	return translate("team editor", err)
}

func (r *postgresTeamEditorRepository) RevokeAllForUser(ctx context.Context, userID string) (int64, error) {
	// No affected() here: removing the grants of a user who held none is a
	// success, not a 404. The caller is reacting to an account deletion, and
	// an account with no grants is the common case.
	res, err := r.db.ExecContext(ctx, `DELETE FROM team_editors WHERE user_id = $1`, userID)
	if err != nil {
		return 0, translate("team editor", err)
	}
	return res.RowsAffected()
}

func (r *postgresTeamEditorRepository) Revoke(ctx context.Context, userID, teamID string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM team_editors WHERE user_id = $1 AND team_id = $2`, userID, teamID)
	return affected("team editor", res, err)
}
