// Package scheduler is the background job that watches subscription dates.
// It runs once at startup and then every Config.Interval, and on each run:
//
//   - an active subscription whose end date is within ExpiringDays becomes
//     expiring_soon; its licence holders and every active admin are warned
//     (subscription_expiring);
//   - an active or expiring_soon subscription whose end date has passed
//     becomes expired; every active admin is alerted (subscription_expired);
//   - a payment due within PaymentDays reminds every active admin
//     (payment_due);
//   - an expiring_soon subscription whose end date was moved out of the
//     window goes back to active, without a notification.
//
// "Today" is the calendar day in Config.Location. The windows come from the
// notification settings, read at the start of every run.
//
// Repeats are safe. Before notifying, the event's key goes into
// notification_log with ON CONFLICT DO NOTHING, and the notification is sent
// only when the key is new. Each subscription is handled in its own
// transaction (key, status change, audit, notifications together), with its
// row locked; a failure on one does not stop the others.
//
// Status changes are recorded in the audit trail as subscription.update with
// no actor and details {"status": ..., "by": "scheduler"}.
package scheduler

import (
	"context"
	"errors"
	"time"

	"subtrack/internal/domain"
	"subtrack/internal/logger"
	"subtrack/internal/storage"
)

// repo is what a run needs from the database; *Store satisfies it.
type repo interface {
	Candidates(ctx context.Context, today, expiringUntil, paymentUntil time.Time) ([]int64, error)
	Lock(ctx context.Context, id int64) (domain.Subscription, error)
	SetStatus(ctx context.Context, id int64, status domain.SubscriptionStatus) error
	MarkSent(ctx context.Context, key string) (bool, error)
	Holders(ctx context.Context, subID int64) ([]int64, error)
}

// settingsReader reads the notification settings; *settings.Store satisfies
// it structurally.
type settingsReader interface {
	Get(ctx context.Context) (domain.NotificationSettings, error)
}

// transactor runs fn in one transaction; *pg.TxRunner satisfies it.
type transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// auditor records a change in the audit trail; *audit.Store satisfies it.
type auditor interface {
	Record(ctx context.Context, e domain.AuditEntry) error
}

// notifier stores in-app notifications; *notification.Store satisfies it.
type notifier interface {
	Send(ctx context.Context, n domain.Notification) error
	SendToAdmins(ctx context.Context, n domain.Notification) error
}

// Scheduler runs the date checks. Build it with New.
type Scheduler struct {
	repo     repo
	settings settingsReader
	tx       transactor
	audit    auditor
	notify   notifier
	cfg      Config
	// now is time.Now; tests replace it.
	now func() time.Time
}

// New wires a Scheduler.
func New(r repo, st settingsReader, tx transactor, a auditor, n notifier, cfg Config) *Scheduler {
	return &Scheduler{repo: r, settings: st, tx: tx, audit: a, notify: n, cfg: cfg, now: time.Now}
}

// Report sums up one run.
type Report struct {
	Checked  int // subscriptions looked at
	Changed  int // status changes
	Notified int // events notified (each may reach several people)
	Failed   int // subscriptions whose transaction failed
}

// Run makes a run now and then every Config.Interval until ctx is
// cancelled. A failed run is logged and the next one happens as usual.
func (s *Scheduler) Run(ctx context.Context) {
	log := logger.Op(ctx, "scheduler.Run")
	log.Info("scheduler started", "interval", s.cfg.Interval, "tz", s.cfg.Location.String())
	t := time.NewTicker(s.cfg.Interval)
	defer t.Stop()
	for {
		rep, err := s.RunOnce(ctx)
		switch {
		case ctx.Err() != nil:
		case err != nil:
			log.Error("scheduler run failed", "err", err)
		case rep.Changed > 0 || rep.Notified > 0 || rep.Failed > 0:
			log.Info("scheduler run done", "checked", rep.Checked, "changed", rep.Changed,
				"notified", rep.Notified, "failed", rep.Failed)
		default:
			log.Debug("scheduler run done", "checked", rep.Checked)
		}
		select {
		case <-ctx.Done():
			log.Info("scheduler stopped")
			return
		case <-t.C:
		}
	}
}

// RunOnce makes one run. A failure on one subscription is logged and counted
// in Report.Failed; the error is only for a run that could not start (no
// settings, no candidate list).
func (s *Scheduler) RunOnce(ctx context.Context) (Report, error) {
	log := logger.Op(ctx, "scheduler.RunOnce")
	st, err := s.settings.Get(ctx)
	if err != nil {
		return Report{}, err
	}
	today := dayIn(s.now(), s.cfg.Location)
	ids, err := s.repo.Candidates(ctx, today,
		today.AddDate(0, 0, st.ExpiringDays), today.AddDate(0, 0, st.PaymentDays))
	if err != nil {
		return Report{}, err
	}
	log.Debug("start", "today", today.Format(time.DateOnly), "candidates", len(ids),
		"expiring_days", st.ExpiringDays, "payment_days", st.PaymentDays)

	var rep Report
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		var changed, notified int
		err := s.tx.InTx(ctx, func(ctx context.Context) error {
			changed, notified = 0, 0
			sub, err := s.repo.Lock(ctx, id)
			if errors.Is(err, storage.ErrNotFound) {
				return nil // deleted since the candidate list was read
			}
			if err != nil {
				return err
			}
			for _, e := range plan(sub, today, st) {
				ch, sent, err := s.apply(ctx, sub, e, today)
				if err != nil {
					return err
				}
				changed += ch
				notified += sent
			}
			return nil
		})
		rep.Checked++
		if err != nil {
			rep.Failed++
			log.Error("scheduler subscription failed", "subscription_id", id, "err", err)
			continue
		}
		rep.Changed += changed
		rep.Notified += notified
	}
	return rep, nil
}

// apply carries out one event inside the subscription's transaction and
// reports how many status changes and notified events it made (0 or 1 each).
func (s *Scheduler) apply(ctx context.Context, sub domain.Subscription, e event, today time.Time) (changed, notified int, err error) {
	log := logger.Op(ctx, "scheduler.apply")
	if e.status != "" {
		if err := s.repo.SetStatus(ctx, sub.ID, e.status); err != nil {
			return 0, 0, err
		}
		if err := s.audit.Record(ctx, domain.AuditEntry{
			Action:     "subscription.update",
			EntityType: "subscription",
			EntityID:   &sub.ID,
			Details:    map[string]any{"status": string(e.status), "by": "scheduler"},
		}); err != nil {
			return 0, 0, err
		}
		log.Info("subscription status changed", "subscription_id", sub.ID, "from", sub.Status, "to", e.status)
		changed = 1
	}
	if e.key == "" {
		return changed, 0, nil
	}
	fresh, err := s.repo.MarkSent(ctx, e.key)
	if err != nil || !fresh {
		return changed, 0, err
	}
	if err := s.send(ctx, sub, e, today); err != nil {
		return 0, 0, err
	}
	log.Info("event notified", "subscription_id", sub.ID, "key", e.key)
	return changed, 1, nil
}

// dateLayout is how dates read in notification texts.
const dateLayout = "02.01.2006"

// send stores the notifications of one event.
func (s *Scheduler) send(ctx context.Context, sub domain.Subscription, e event, today time.Time) error {
	switch e.kind {
	case evExpiring:
		n := domain.Notification{
			Type:  domain.NotifySubscriptionExpiring,
			Title: "Подписка скоро закончится: " + sub.Name,
			Body: "Подписка «" + sub.Name + "» действует до " + sub.EndDate.Format(dateLayout) +
				" включительно — это " + inDays(daysUntil(today, *sub.EndDate)) + ".",
		}
		holders, err := s.repo.Holders(ctx, sub.ID)
		if err != nil {
			return err
		}
		for _, uid := range holders {
			n.UserID = uid
			if err := s.notify.Send(ctx, n); err != nil {
				return err
			}
		}
		return s.notify.SendToAdmins(ctx, n)
	case evExpired:
		return s.notify.SendToAdmins(ctx, domain.Notification{
			Type:  domain.NotifySubscriptionExpired,
			Title: "Подписка закончилась: " + sub.Name,
			Body: "Срок подписки «" + sub.Name + "» истёк " + sub.EndDate.Format(dateLayout) +
				", статус изменён на «истекла». Продлите её или отмените.",
		})
	case evPayment:
		return s.notify.SendToAdmins(ctx, domain.Notification{
			Type:  domain.NotifyPaymentDue,
			Title: "Скоро платёж: " + sub.Name,
			Body: "Платёж по подписке «" + sub.Name + "» — " + sub.NextPaymentDate.Format(dateLayout) +
				" (" + inDays(daysUntil(today, *sub.NextPaymentDate)) + "), сумма " + sub.Cost + " " + sub.Currency + ".",
		})
	}
	return nil
}
