// Package settings holds the company-wide notification rules: how many days
// before a subscription ends its holders are warned, and how many days before
// a payment the admins are reminded. Only the admin reads and changes them
// (see the routes); the scheduler reads them through *Store.Get at the start
// of every run, so a change takes effect without a restart.
package settings

import (
	"context"
	"fmt"

	"subtrack/internal/apperr"
	"subtrack/internal/domain"
	"subtrack/internal/logger"
)

// ErrInvalidDays is returned when a value is outside 0..domain.MaxNotifyDays.
// Mapped to 400.
var ErrInvalidDays = apperr.New(apperr.ErrInvalidInput,
	fmt.Sprintf("service: invalid input: days must be between 0 and %d", domain.MaxNotifyDays))

// repo is what the service needs; *Store satisfies it.
type repo interface {
	Get(ctx context.Context) (domain.NotificationSettings, error)
	Update(ctx context.Context, st domain.NotificationSettings, actorID int64) (domain.NotificationSettings, error)
}

// transactor runs fn in one transaction; *pg.TxRunner satisfies it.
type transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// auditor records a change in the audit trail; *audit.Store satisfies it.
type auditor interface {
	Record(ctx context.Context, e domain.AuditEntry) error
}

// Service implements reading and changing the notification settings.
type Service struct {
	repo  repo
	tx    transactor
	audit auditor
}

// New wires a Service.
func New(r repo, tx transactor, a auditor) *Service {
	return &Service{repo: r, tx: tx, audit: a}
}

// Get returns the current settings.
func (s *Service) Get(ctx context.Context) (domain.NotificationSettings, error) {
	return s.repo.Get(ctx)
}

// Update replaces both values and records the change in the audit trail, in
// one transaction. Access is limited to the admin by the routes.
//
// Errors:
//   - ErrInvalidDays when a value is out of range.
//   - any other datastore failure, propagated.
func (s *Service) Update(ctx context.Context, actor domain.User, expiringDays, paymentDays int) (domain.NotificationSettings, error) {
	log := logger.Op(ctx, "settings.Service.Update")
	if !validDays(expiringDays) || !validDays(paymentDays) {
		return domain.NotificationSettings{}, ErrInvalidDays
	}
	in := domain.NotificationSettings{ExpiringDays: expiringDays, PaymentDays: paymentDays}

	var out domain.NotificationSettings
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if out, err = s.repo.Update(ctx, in, actor.ID); err != nil {
			return err
		}
		return s.audit.Record(ctx, domain.AuditEntry{
			ActorID:    &actor.ID,
			Action:     "settings.update",
			EntityType: "settings",
			Details:    map[string]any{"expiring_days": out.ExpiringDays, "payment_days": out.PaymentDays},
		})
	})
	if err != nil {
		return domain.NotificationSettings{}, err
	}
	log.Info("notification settings updated", "expiring_days", out.ExpiringDays, "payment_days", out.PaymentDays, "by", actor.ID)
	return out, nil
}

func validDays(n int) bool {
	return n >= 0 && n <= domain.MaxNotifyDays
}
