package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/apperr"
)

// MaxNoteLength bounds a note.
//
// Generous on purpose: a scouting note is a paragraph or three, and a limit
// that cuts someone off mid-thought makes the feature worse without making the
// database safer. What it does stop is a request that is really a file upload.
//
// Counted in runes, so a note in Turkish or Greek is measured the way its
// writer counts it rather than in bytes.
const MaxNoteLength = 4000

// PlayerNoteService is the members' commentary on a player.
//
// The rules are deliberately different from the rest of this service. Editing
// a player is for editors and administrators; writing a note is for **any
// signed-in member**, because the point is a range of opinions. What keeps it
// from becoming a free-for-all is that each person gets exactly one note, which
// they may rewrite as often as they like.
type PlayerNoteService interface {
	ListByPlayer(ctx context.Context, playerID string, page domain.Page) ([]domain.PlayerNote, error)
	// Mine returns the caller's own note on a player, or not-found.
	Mine(ctx context.Context, playerID string) (*domain.PlayerNote, error)
	Write(ctx context.Context, playerID, body string) (*domain.PlayerNote, error)
	Edit(ctx context.Context, noteID, body string) (*domain.PlayerNote, error)
	Delete(ctx context.Context, noteID string) error
}

type playerNoteService struct {
	repo    repository.PlayerNoteRepository
	players repository.PlayerRepository
	authz   *Authorizer
}

func NewPlayerNoteService(
	repo repository.PlayerNoteRepository,
	players repository.PlayerRepository,
	authz *Authorizer,
) PlayerNoteService {
	return &playerNoteService{repo: repo, players: players, authz: authz}
}

// ListByPlayer is a public read: notes are the product, and hiding them from
// signed-out visitors would make the page worth less than the sum of its data.
func (s *playerNoteService) ListByPlayer(ctx context.Context, playerID string, page domain.Page) ([]domain.PlayerNote, error) {
	if playerID == "" {
		return nil, apperr.Invalid("player id is required")
	}
	return s.repo.ListByPlayer(ctx, playerID, page)
}

func (s *playerNoteService) Mine(ctx context.Context, playerID string) (*domain.PlayerNote, error) {
	claims, err := s.claims(ctx)
	if err != nil {
		return nil, err
	}
	return s.repo.GetByAuthor(ctx, playerID, claims.UserID)
}

// Write records the caller's note.
//
// A second attempt is refused by the unique constraint rather than silently
// overwriting: someone who has already written one should see their existing
// text and edit it, not discover it was replaced.
func (s *playerNoteService) Write(ctx context.Context, playerID, body string) (*domain.PlayerNote, error) {
	claims, err := s.claims(ctx)
	if err != nil {
		return nil, err
	}

	trimmed, err := validateNoteBody(body)
	if err != nil {
		return nil, err
	}

	// Resolve the player first, so an unknown id is a clean 404 naming what is
	// missing rather than a foreign-key violation.
	if _, err := s.players.GetByID(ctx, playerID); err != nil {
		return nil, err
	}

	note := &domain.PlayerNote{
		PlayerID: playerID,
		AuthorID: claims.UserID,
		// From the token, never from the request body: a client-supplied name
		// would let anyone sign a note as somebody else.
		AuthorName: claims.Username,
		Body:       trimmed,
	}
	if err := s.repo.Create(ctx, note); err != nil {
		if apperr.KindOf(err) == apperr.KindConflict {
			return nil, apperr.Conflict("you have already written a note on this player; edit that one instead")
		}
		return nil, err
	}
	return note, nil
}

// Edit rewrites a note. Only its author, or an administrator.
//
// An administrator may edit rather than only delete because moderation is
// usually a matter of removing one sentence, and deleting somebody's whole
// note to do it is a blunter instrument than the situation needs.
func (s *playerNoteService) Edit(ctx context.Context, noteID, body string) (*domain.PlayerNote, error) {
	existing, err := s.repo.GetByID(ctx, noteID)
	if err != nil {
		return nil, err
	}
	if err := s.requireAuthorOrAdmin(ctx, existing.AuthorID); err != nil {
		return nil, err
	}

	trimmed, err := validateNoteBody(body)
	if err != nil {
		return nil, err
	}
	return s.repo.Update(ctx, noteID, trimmed)
}

func (s *playerNoteService) Delete(ctx context.Context, noteID string) error {
	existing, err := s.repo.GetByID(ctx, noteID)
	if err != nil {
		return err
	}
	if err := s.requireAuthorOrAdmin(ctx, existing.AuthorID); err != nil {
		return err
	}
	return s.repo.Delete(ctx, noteID)
}

// requireAuthorOrAdmin allows the note's own author, and administrators for
// moderation. Editor grants are irrelevant here: holding a club does not make
// someone the author of another member's opinion.
func (s *playerNoteService) requireAuthorOrAdmin(ctx context.Context, authorID string) error {
	claims, err := s.claims(ctx)
	if err != nil {
		return err
	}
	if claims.UserID == authorID || claims.Role == RoleAdmin {
		return nil
	}
	return ErrForbidden
}

func (s *playerNoteService) claims(ctx context.Context) (*auth.Claims, error) {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	return claims, nil
}

// validateNoteBody trims and bounds the text.
//
// Trimming first means a note of nothing but whitespace is empty rather than
// being stored and rendered as a blank comment.
func validateNoteBody(body string) (string, error) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", apperr.Invalid("a note cannot be empty")
	}
	if utf8.RuneCountInString(trimmed) > MaxNoteLength {
		return "", apperr.Invalid("a note can be at most 4000 characters")
	}
	return trimmed, nil
}
