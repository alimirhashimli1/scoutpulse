package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"math/big"
	"strings"
	"unicode/utf8"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/auth"
	"github.com/scoutpulse/libs/platform/apperr"
)

// Bounds on a board.
//
// MaxTacticContentBytes is far above what the editor produces -- a busy
// animated drill is tens of kilobytes -- and well below the 1 MiB the HTTP
// layer accepts. What it stops is a request that is really a file upload.
const (
	MaxTacticTitleLength       = 120
	MaxTacticDescriptionLength = 1000
	MaxTacticContentBytes      = 512 << 10

	// MaxTacticBoardsPerOwner bounds what one account can store. Any member
	// may create boards, and accounts are free, so without it the table is
	// open-ended storage for anyone who signs up. Far above what a coach
	// keeps; low enough that the worst case is a few hundred megabytes.
	MaxTacticBoardsPerOwner = 500
)

// slugAlphabet leaves out characters that are easy to misread when a link is
// copied by hand: 0/O, 1/l/I.
const slugAlphabet = "23456789abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ"

// slugLength gives about 70 bits of randomness, so a board cannot be found by
// guessing its address.
const slugLength = 12

// TacticService is members' tactics boards.
//
// The rules differ from the rest of this service in the same way member notes
// do: **any signed-in member** may create boards, since a coach's drill is not
// an edit to the shared record. Boards are unlisted: a public link plays one,
// and the list shows each member only their own -- signing up is free, so a
// list open to every member would be open to everyone.
type TacticService interface {
	// Get is public. Holding the slug is what grants access.
	Get(ctx context.Context, slug string) (*domain.TacticBoard, error)
	// List returns the caller's own boards. Only an administrator may list
	// everyone's, or another author's by ownerID.
	List(ctx context.Context, ownerID *string, page domain.Page) ([]domain.TacticSummary, error)
	Create(ctx context.Context, input TacticInput) (*domain.TacticBoard, error)
	Update(ctx context.Context, slug string, input TacticInput) (*domain.TacticBoard, error)
	Delete(ctx context.Context, slug string) error
}

// TacticInput is the writable surface of a board. The owner comes from the
// token and the slug from the server, never from the request.
type TacticInput struct {
	Title       string
	Description string
	Content     json.RawMessage
}

type tacticService struct {
	repo repository.TacticRepository
	// newSlug is swappable so a test can force a collision.
	newSlug func() (string, error)
}

func NewTacticService(repo repository.TacticRepository) TacticService {
	return &tacticService{repo: repo, newSlug: randomSlug}
}

func (s *tacticService) Get(ctx context.Context, slug string) (*domain.TacticBoard, error) {
	if !validSlug(slug) {
		// Not a lookup that could succeed, so answer it without one. Also keeps
		// arbitrary path text out of the query log.
		return nil, apperr.NotFound("tactics board not found")
	}
	return s.repo.GetBySlug(ctx, slug)
}

// List requires an account and, for anyone but an administrator, returns only
// the caller's boards whatever ownerID asks for. A board is reachable by its
// link; a list of other members' links would make every board public to
// anyone willing to register.
func (s *tacticService) List(ctx context.Context, ownerID *string, page domain.Page) ([]domain.TacticSummary, error) {
	claims, err := s.claims(ctx)
	if err != nil {
		return nil, err
	}
	if claims.Role != RoleAdmin {
		ownerID = &claims.UserID
	}
	return s.repo.List(ctx, ownerID, page)
}

// Create stores a new board under the caller's name.
//
// The slug is drawn here and retried on the rare collision, rather than
// checked for first: two creates could both pass a check, and only the unique
// constraint can decide between them.
func (s *tacticService) Create(ctx context.Context, input TacticInput) (*domain.TacticBoard, error) {
	claims, err := s.claims(ctx)
	if err != nil {
		return nil, err
	}

	board, err := validateTactic(input)
	if err != nil {
		return nil, err
	}
	board.OwnerID = claims.UserID
	// From the token, never from the request: a client-supplied name would let
	// anyone file a board under somebody else's.
	board.OwnerName = claims.Username

	// Checked before the insert rather than enforced by it, so two creates
	// racing at the limit can both land. That overshoot is one board, which
	// is not what the cap is for.
	count, err := s.repo.CountByOwner(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	if count >= MaxTacticBoardsPerOwner {
		return nil, apperr.Invalid("you have reached the limit of 500 tactics boards; delete some to make room")
	}

	const attempts = 3
	for i := 0; i < attempts; i++ {
		if board.Slug, err = s.newSlug(); err != nil {
			return nil, apperr.Internal(err)
		}
		err = s.repo.Create(ctx, board)
		if err == nil {
			return board, nil
		}
		if apperr.KindOf(err) != apperr.KindConflict {
			return nil, err
		}
	}
	return nil, err
}

// Update rewrites a board. Only its author.
//
// Unlike a note, an administrator may not edit someone else's board: a
// drawing cannot be moderated a sentence at a time, so delete is the whole
// moderation tool here.
func (s *tacticService) Update(ctx context.Context, slug string, input TacticInput) (*domain.TacticBoard, error) {
	claims, err := s.claims(ctx)
	if err != nil {
		return nil, err
	}

	existing, err := s.Get(ctx, slug)
	if err != nil {
		return nil, err
	}
	if existing.OwnerID != claims.UserID {
		return nil, ErrForbidden
	}

	updated, err := validateTactic(input)
	if err != nil {
		return nil, err
	}
	existing.Title = updated.Title
	existing.Description = updated.Description
	existing.Content = updated.Content

	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// Delete removes a board. Its author, or an administrator for moderation.
func (s *tacticService) Delete(ctx context.Context, slug string) error {
	claims, err := s.claims(ctx)
	if err != nil {
		return err
	}

	existing, err := s.Get(ctx, slug)
	if err != nil {
		return err
	}
	if existing.OwnerID != claims.UserID && claims.Role != RoleAdmin {
		return ErrForbidden
	}
	return s.repo.Delete(ctx, slug)
}

func (s *tacticService) claims(ctx context.Context) (*auth.Claims, error) {
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	return claims, nil
}

// validateTactic trims and bounds the fields and checks the document is a JSON
// object. It returns a board carrying only the validated input.
func validateTactic(input TacticInput) (*domain.TacticBoard, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, apperr.Invalid("a tactics board needs a title")
	}
	if utf8.RuneCountInString(title) > MaxTacticTitleLength {
		return nil, apperr.Invalid("a title can be at most 120 characters")
	}

	description := strings.TrimSpace(input.Description)
	if utf8.RuneCountInString(description) > MaxTacticDescriptionLength {
		return nil, apperr.Invalid("a description can be at most 1000 characters")
	}

	content := bytes.TrimSpace(input.Content)
	if len(content) == 0 {
		return nil, apperr.Invalid("the board is empty")
	}
	if len(content) > MaxTacticContentBytes {
		return nil, apperr.Invalid("the board is too large to save; remove some steps or drawings")
	}
	// The JSON decoder has already proved it is valid JSON. What it has not
	// proved is that it is an object rather than a string or an array, which
	// the table's CHECK would otherwise report as a constraint name.
	if content[0] != '{' {
		return nil, apperr.Invalid("the board must be a JSON object")
	}

	return &domain.TacticBoard{
		Title:       title,
		Description: description,
		Content:     json.RawMessage(content),
	}, nil
}

func randomSlug() (string, error) {
	max := big.NewInt(int64(len(slugAlphabet)))
	out := make([]byte, slugLength)
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = slugAlphabet[n.Int64()]
	}
	return string(out), nil
}

func validSlug(slug string) bool {
	if len(slug) != slugLength {
		return false
	}
	for i := 0; i < len(slug); i++ {
		if !strings.ContainsRune(slugAlphabet, rune(slug[i])) {
			return false
		}
	}
	return true
}
