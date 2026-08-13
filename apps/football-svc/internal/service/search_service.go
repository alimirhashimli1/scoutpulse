package service

import (
	"context"
	"strings"

	"github.com/scoutpulse/football-svc/internal/domain"
	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/platform/apperr"
)

// MinSearchQueryLength is the shortest query accepted.
//
// A single character matches a large fraction of the dataset and tells the
// user nothing, while still costing a full index scan. Two is the point where
// a prefix search starts to discriminate.
const MinSearchQueryLength = 2

type SearchService interface {
	Search(ctx context.Context, query string, kind domain.SearchKind, page domain.Page) ([]domain.SearchResult, error)
}

type searchService struct {
	repo repository.SearchRepository
}

func NewSearchService(repo repository.SearchRepository) SearchService {
	return &searchService{repo: repo}
}

// Search is a public read. Everything it can return is already public through
// the entity endpoints, so requiring a token would only stop a search box
// working before login.
func (s *searchService) Search(ctx context.Context, query string, kind domain.SearchKind, page domain.Page) ([]domain.SearchResult, error) {
	query = strings.TrimSpace(query)

	if len([]rune(query)) < MinSearchQueryLength {
		return nil, apperr.Invalid("q must be at least 2 characters")
	}
	if !domain.ValidSearchKind(kind) {
		return nil, apperr.Invalid("kind must be one of: player, team, coach, league")
	}

	return s.repo.Search(ctx, query, kind, page)
}
