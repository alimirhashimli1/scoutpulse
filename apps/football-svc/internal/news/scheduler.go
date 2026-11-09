package news

import (
	"context"
	"log/slog"
	"time"
)

// DefaultInterval is how often the pipeline runs.
const DefaultInterval = 30 * time.Minute

// StartScheduler runs the pipeline every interval until ctx ends. The first
// run waits a minute, so a deploy's boot is not also a fetch, and a restart
// loop cannot hammer the sources.
func StartScheduler(ctx context.Context, p *Pipeline, interval time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		interval = DefaultInterval
	}
	go func() {
		first := time.NewTimer(time.Minute)
		defer first.Stop()
		select {
		case <-ctx.Done():
			return
		case <-first.C:
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			report, err := p.Run(ctx)
			switch {
			case err != nil:
				logger.Error("news run failed", "error", err)
			case report.Skipped != "":
				logger.Info("news run skipped", "reason", report.Skipped)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
