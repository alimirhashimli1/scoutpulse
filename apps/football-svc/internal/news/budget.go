package news

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// UsageStore keeps daily counters. The day is UTC, which is when Cloudflare
// and NewsData reset their allowances.
type UsageStore interface {
	Usage(ctx context.Context, day time.Time, meter string) (int, error)
	AddUsage(ctx context.Context, day time.Time, meter string, amount int) error
}

// ErrBudgetExhausted means a daily allowance would be exceeded.
var ErrBudgetExhausted = errors.New("daily budget used up")

// Budget enforces a daily ceiling on one meter. Reserve records the spend
// before the work happens, so a crash mid-call can only under-use the
// allowance, never over-use it.
type Budget struct {
	Store UsageStore
	Meter string
	Limit int
	Now   func() time.Time

	mu sync.Mutex
}

func (b *Budget) day() time.Time {
	now := time.Now
	if b.Now != nil {
		now = b.Now
	}
	t := now().UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// Remaining reports what is left today.
func (b *Budget) Remaining(ctx context.Context) (int, error) {
	used, err := b.Store.Usage(ctx, b.day(), b.Meter)
	if err != nil {
		return 0, err
	}
	return max(0, b.Limit-used), nil
}

// Reserve claims n units of today's allowance or fails without claiming any.
func (b *Budget) Reserve(ctx context.Context, n int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	left, err := b.Remaining(ctx)
	if err != nil {
		return err
	}
	if n > left {
		return fmt.Errorf("%s: %w (%d left, %d needed)", b.Meter, ErrBudgetExhausted, left, n)
	}
	return b.Store.AddUsage(ctx, b.day(), b.Meter, n)
}

// Adjust corrects a reservation by delta once the real cost is known. A
// refund never takes the counter below what was really spent.
func (b *Budget) Adjust(ctx context.Context, delta int) error {
	if delta == 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Store.AddUsage(ctx, b.day(), b.Meter, delta)
}

// Meters.
const (
	MeterNeurons         = "ai_neurons"
	MeterNewsDataCredits = "newsdata_credits"
)

// Ceilings. The AI ceiling sits under Cloudflare's free 10,000 Neurons a day;
// NewsData's sits under its free 200 credits. Configuration can lower them,
// never raise them past these.
const (
	MaxNeuronsPerDay         = 9000
	DefaultNeuronsPerDay     = 8000
	MaxNewsDataCreditsPerDay = 180
	DefaultNewsDataCredits   = 150
)
