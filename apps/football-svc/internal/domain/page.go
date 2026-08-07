package domain

// Paging bounds. MaxPageSize is the ceiling a client can request; without it
// a single call could ask for the entire players table.
const (
	DefaultPageSize = 25
	MaxPageSize     = 100
)

// Page is a limit/offset window over a list query.
type Page struct {
	Limit  int
	Offset int
}

// NewPage clamps caller-supplied paging parameters into a safe range. A
// non-positive limit falls back to DefaultPageSize; a negative offset is
// treated as zero.
func NewPage(limit, offset int) Page {
	if limit <= 0 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return Page{Limit: limit, Offset: offset}
}

// FetchLimit is the number of rows a repository should request: one more than
// the page size, so the extra row reveals whether another page exists without
// a second COUNT query.
func (p Page) FetchLimit() int { return p.Limit + 1 }

// ListResult is the response envelope for every paginated endpoint.
type ListResult[T any] struct {
	Items   []T  `json:"items"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}

// NewListResult trims the sentinel row fetched by FetchLimit and reports
// whether more results are available.
func NewListResult[T any](items []T, p Page) ListResult[T] {
	hasMore := len(items) > p.Limit
	if hasMore {
		items = items[:p.Limit]
	}
	if items == nil {
		// Serialize as [] rather than null so clients can iterate blindly.
		items = []T{}
	}
	return ListResult[T]{
		Items:   items,
		Limit:   p.Limit,
		Offset:  p.Offset,
		HasMore: hasMore,
	}
}
