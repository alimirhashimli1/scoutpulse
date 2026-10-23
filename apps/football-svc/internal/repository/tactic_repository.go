package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/libs/platform/apperr"
)

// TacticRepository stores members' tactics boards.
type TacticRepository interface {
	GetBySlug(ctx context.Context, slug string) (*domain.TacticBoard, error)
	// List returns summaries grouped by author -- ordered by author name, then
	// newest first within each author -- optionally narrowed to one author.
	List(ctx context.Context, ownerID *string, page domain.Page) ([]domain.TacticSummary, error)
	Create(ctx context.Context, board *domain.TacticBoard) error
	Update(ctx context.Context, board *domain.TacticBoard) error
	Delete(ctx context.Context, slug string) error
	// CountByOwner reports how many boards one account holds.
	CountByOwner(ctx context.Context, ownerID string) (int, error)
	// DeleteByOwner removes every board by one account, for when that account
	// is deleted. Boards are keyed by a user id this database cannot enforce a
	// foreign key against.
	DeleteByOwner(ctx context.Context, ownerID string) (int64, error)
}

type postgresTacticRepository struct {
	db *sqlx.DB
}

func NewPostgresTacticRepository(db *sqlx.DB) TacticRepository {
	return &postgresTacticRepository{db: db}
}

const tacticColumns = `id, slug, owner_id, owner_name, title, description, content, created_at, updated_at`

// tacticSummaryColumns leaves the document behind and counts its steps
// instead. coalesce covers a document with no steps array, which the service
// does not forbid.
const tacticSummaryColumns = `slug, owner_id, owner_name, title, description,
	coalesce(jsonb_array_length(CASE WHEN jsonb_typeof(content->'steps') = 'array'
	                                 THEN content->'steps' END), 0) AS steps,
	created_at, updated_at`

func (r *postgresTacticRepository) GetBySlug(ctx context.Context, slug string) (*domain.TacticBoard, error) {
	var board domain.TacticBoard
	query := `SELECT ` + tacticColumns + ` FROM tactic_boards WHERE slug = $1`
	if err := r.db.GetContext(ctx, &board, query, slug); err != nil {
		return nil, translate("tactics board", err)
	}
	return &board, nil
}

// List orders by author first, because the members' page shows one section
// per author. owner_id breaks ties between two accounts that once had the same
// name, so their boards do not interleave.
func (r *postgresTacticRepository) List(ctx context.Context, ownerID *string, page domain.Page) ([]domain.TacticSummary, error) {
	var boards []domain.TacticSummary
	query := `SELECT ` + tacticSummaryColumns + ` FROM tactic_boards
	           WHERE ($1::uuid IS NULL OR owner_id = $1::uuid)
	           ORDER BY lower(owner_name), owner_id, updated_at DESC, id
	           LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &boards, query, ownerID, page.FetchLimit(), page.Offset); err != nil {
		return nil, translate("tactics board", err)
	}
	return boards, nil
}

// Create inserts a board. A slug collision surfaces as a conflict, which the
// service answers by drawing another.
//
// The document is passed as a string: lib/pq sends a []byte as bytea, which
// Postgres will not read as JSON.
func (r *postgresTacticRepository) Create(ctx context.Context, board *domain.TacticBoard) error {
	query := `INSERT INTO tactic_boards (slug, owner_id, owner_name, title, description, content)
	          VALUES ($1, $2, $3, $4, $5, $6)
	          RETURNING id, created_at, updated_at`
	err := r.db.QueryRowContext(ctx, query,
		board.Slug, board.OwnerID, board.OwnerName, board.Title, board.Description, string(board.Content),
	).Scan(&board.ID, &board.CreatedAt, &board.UpdatedAt)
	if err != nil {
		return translate("tactics board", err)
	}
	return nil
}

// Update rewrites the title, description and document. The owner and the slug
// never change: the link someone was sent keeps working after every edit.
func (r *postgresTacticRepository) Update(ctx context.Context, board *domain.TacticBoard) error {
	query := `UPDATE tactic_boards
	             SET title = $1, description = $2, content = $3, updated_at = NOW()
	           WHERE slug = $4
	       RETURNING updated_at`
	err := r.db.QueryRowContext(ctx, query,
		board.Title, board.Description, string(board.Content), board.Slug,
	).Scan(&board.UpdatedAt)
	if err != nil {
		return translate("tactics board", err)
	}
	return nil
}

func (r *postgresTacticRepository) Delete(ctx context.Context, slug string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM tactic_boards WHERE slug = $1`, slug)
	return affected("tactics board", res, err)
}

func (r *postgresTacticRepository) CountByOwner(ctx context.Context, ownerID string) (int, error) {
	var n int
	if err := r.db.GetContext(ctx, &n, `SELECT count(*) FROM tactic_boards WHERE owner_id = $1`, ownerID); err != nil {
		return 0, translate("tactics board", err)
	}
	return n, nil
}

func (r *postgresTacticRepository) DeleteByOwner(ctx context.Context, ownerID string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM tactic_boards WHERE owner_id = $1`, ownerID)
	if err != nil {
		return 0, apperr.Wrap(apperr.KindInternal, "could not remove the account's tactics boards", err)
	}
	return res.RowsAffected()
}
