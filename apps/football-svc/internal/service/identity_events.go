package service

import (
	"context"
	"log/slog"

	"github.com/scoutpulse/football-svc/internal/repository"
	"github.com/scoutpulse/libs/platform/events"
)

// IdentityEventConsumer keeps this service's user-keyed data in step with the
// identity service.
//
// team_editors stores grants against a user_id with no foreign key, because
// users live in another service's database and no constraint can cross that
// boundary. Nothing would otherwise tell this service an account is gone, so
// grants would outlive it — and if the id were ever reissued they would apply
// to whoever received it. This consumer is what closes that (issue N16).
type IdentityEventConsumer struct {
	editors repository.TeamEditorRepository
	authz   *Authorizer
	logger  *slog.Logger
}

func NewIdentityEventConsumer(
	editors repository.TeamEditorRepository,
	authz *Authorizer,
	logger *slog.Logger,
) *IdentityEventConsumer {
	if logger == nil {
		logger = slog.Default()
	}
	return &IdentityEventConsumer{editors: editors, authz: authz, logger: logger}
}

// Register subscribes to the identity events this service cares about.
//
// The queue group means that with several replicas running, exactly one of
// them handles each event rather than all of them doing the same delete.
func (c *IdentityEventConsumer) Register(ctx context.Context, sub events.Subscriber) error {
	if err := sub.Subscribe(ctx, events.SubjectUserDeleted, "football-svc", c.handleUserDeleted); err != nil {
		return err
	}
	return sub.Subscribe(ctx, events.SubjectUserRoleChanged, "football-svc", c.handleRoleChanged)
}

// handleUserDeleted drops every grant the account held.
func (c *IdentityEventConsumer) handleUserDeleted(ctx context.Context, e events.Envelope) error {
	var payload events.UserDeleted
	if err := e.Decode(&payload); err != nil {
		// A malformed payload will never become well-formed on redelivery, so
		// returning an error would only produce an endless retry. Log and drop.
		c.logger.Error("dropping malformed user.deleted event", "event_id", e.ID, "error", err)
		return nil
	}
	if payload.UserID == "" {
		return nil
	}

	removed, err := c.editors.RevokeAllForUser(ctx, payload.UserID)
	if err != nil {
		// A real failure is worth retrying: the grants must not survive.
		return err
	}

	// Drop the cached authorization answers too, so the removal is immediate
	// rather than taking effect at the end of the cache TTL.
	c.authz.InvalidateUser(payload.UserID)

	if removed > 0 {
		c.logger.Info("removed editor grants for a deleted account",
			"user_id", payload.UserID, "username", payload.Username, "grants", removed)
	}
	return nil
}

// handleRoleChanged drops cached grants for a user whose role changed.
//
// The grant itself is unaffected by a role change, but the cached *decision*
// combines the grant with the role from the token. Clearing it means the next
// request re-reads both rather than serving an answer computed under the old
// role.
func (c *IdentityEventConsumer) handleRoleChanged(ctx context.Context, e events.Envelope) error {
	var payload events.UserRoleChanged
	if err := e.Decode(&payload); err != nil {
		c.logger.Error("dropping malformed user.role_changed event", "event_id", e.ID, "error", err)
		return nil
	}
	if payload.UserID == "" {
		return nil
	}

	c.authz.InvalidateUser(payload.UserID)
	c.logger.Info("cleared cached grants after a role change",
		"user_id", payload.UserID, "old_role", payload.OldRole, "new_role", payload.NewRole)
	return nil
}
