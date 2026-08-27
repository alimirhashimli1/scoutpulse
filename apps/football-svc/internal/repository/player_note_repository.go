package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/libs/platform/apperr"
)

// PlayerNoteRepository stores one note per member per player.
type PlayerNoteRepository interface {
	ListByPlayer(ctx context.Context, playerID string, page domain.Page) ([]domain.PlayerNote, error)
	// GetByAuthor returns this author's note on a player, or not-found.
	GetByAuthor(ctx context.Context, playerID, authorID string) (*domain.PlayerNote, error)
	GetByID(ctx context.Context, id string) (*domain.PlayerNote, error)
	Create(ctx context.Context, note *domain.PlayerNote) error
	Update(ctx context.Context, id, body string) (*domain.PlayerNote, error)
	Delete(ctx context.Context, id string) error
	// DeleteByAuthor removes every note by one account, for when that account
	// is deleted. Notes are keyed by a user id this database cannot enforce a
	// foreign key against.
	DeleteByAuthor(ctx context.Context, authorID string) (int64, error)
}

type postgresPlayerNoteRepository struct {
	db *sqlx.DB
}

func NewPlayerNoteRepository(db *sqlx.DB) PlayerNoteRepository {
	return &postgresPlayerNoteRepository{db: db}
}

const noteColumns = `id, player_id, author_id, author_name, body, created_at, updated_at`

// ListByPlayer returns notes newest first.
//
// Newest first because a note is a comment, and the recent opinion is the one
// worth reading -- unlike a career, where the order is the story.
func (r *postgresPlayerNoteRepository) ListByPlayer(ctx context.Context, playerID string, page domain.Page) ([]domain.PlayerNote, error) {
	var notes []domain.PlayerNote
	query := `SELECT ` + noteColumns + ` FROM player_notes
	           WHERE player_id = $1
	           ORDER BY created_at DESC, id
	           LIMIT $2 OFFSET $3`
	if err := r.db.SelectContext(ctx, &notes, query, playerID, page.FetchLimit(), page.Offset); err != nil {
		return nil, translate("note", err)
	}
	return notes, nil
}

func (r *postgresPlayerNoteRepository) GetByAuthor(ctx context.Context, playerID, authorID string) (*domain.PlayerNote, error) {
	var note domain.PlayerNote
	query := `SELECT ` + noteColumns + ` FROM player_notes WHERE player_id = $1 AND author_id = $2`
	if err := r.db.GetContext(ctx, &note, query, playerID, authorID); err != nil {
		return nil, translate("note", err)
	}
	return &note, nil
}

func (r *postgresPlayerNoteRepository) GetByID(ctx context.Context, id string) (*domain.PlayerNote, error) {
	var note domain.PlayerNote
	query := `SELECT ` + noteColumns + ` FROM player_notes WHERE id = $1`
	if err := r.db.GetContext(ctx, &note, query, id); err != nil {
		return nil, translate("note", err)
	}
	return &note, nil
}

// Create inserts a note.
//
// A second note by the same author on the same player violates the unique
// constraint and surfaces as a conflict, which is the intended answer: the
// caller should edit the one they have. Relying on the constraint rather than
// a prior existence check is what makes two simultaneous posts impossible --
// both would pass the check, only one can pass the constraint.
func (r *postgresPlayerNoteRepository) Create(ctx context.Context, note *domain.PlayerNote) error {
	query := `INSERT INTO player_notes (player_id, author_id, author_name, body)
	          VALUES ($1, $2, $3, $4)
	          RETURNING id, created_at, updated_at`
	err := r.db.QueryRowContext(ctx, query,
		note.PlayerID, note.AuthorID, note.AuthorName, note.Body,
	).Scan(&note.ID, &note.CreatedAt, &note.UpdatedAt)
	if err != nil {
		return translate("note", err)
	}
	return nil
}

func (r *postgresPlayerNoteRepository) Update(ctx context.Context, id, body string) (*domain.PlayerNote, error) {
	var note domain.PlayerNote
	// updated_at moves, created_at does not: the list stays ordered by when
	// someone first commented, so editing a note cannot be used to push it back
	// to the top of the page.
	query := `UPDATE player_notes SET body = $1, updated_at = NOW()
	           WHERE id = $2
	       RETURNING ` + noteColumns
	if err := r.db.GetContext(ctx, &note, query, body, id); err != nil {
		return nil, translate("note", err)
	}
	return &note, nil
}

func (r *postgresPlayerNoteRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM player_notes WHERE id = $1`, id)
	return affected("note", res, err)
}

func (r *postgresPlayerNoteRepository) DeleteByAuthor(ctx context.Context, authorID string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM player_notes WHERE author_id = $1`, authorID)
	if err != nil {
		return 0, apperr.Wrap(apperr.KindInternal, "could not remove the account's notes", err)
	}
	return res.RowsAffected()
}
