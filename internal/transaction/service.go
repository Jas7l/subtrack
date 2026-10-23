// Package transaction is the payment history of subscriptions, served to the
// admin only.
//
// Payments come from two places:
//   - the scheduler creates a pending one on each next_payment_date, for the
//     subscription's cost at that moment, and moves next_payment_date one
//     billing period on (see package scheduler);
//   - the admin enters one by hand (a one-off purchase, past payments); it is
//     paid at once and its amount is what the admin says.
//
// Statuses: pending → paid | failed, failed → paid; paid is final. A retry
// after a failure is the same row: a subscription has one payment per date.
// The amount of a payment is never changed.
//
// Every change is recorded in the audit trail in the same transaction.
package transaction

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/domain"
	"subtrack/internal/logger"
	"subtrack/internal/paging"
	"subtrack/internal/storage"
)

var (
	// ErrTransactionNotFound is returned when no payment has that id. Mapped
	// to 404.
	ErrTransactionNotFound = apperr.New(apperr.ErrNotFound, "service: transaction not found")

	// ErrBadTransition is returned for a status change the rules do not
	// allow (paying a paid one, failing a failed one). Mapped to 409.
	ErrBadTransition = apperr.New(apperr.ErrConflict, "service: the transaction cannot move to that status")

	// ErrDuplicateDate is returned when the subscription already has a
	// payment on that date. Mapped to 409.
	ErrDuplicateDate = apperr.New(apperr.ErrConflict, "service: the subscription already has a payment on that date")

	// ErrUnknownSubscription is returned for a subscription_id with no live
	// subscription. Mapped to 400.
	ErrUnknownSubscription = apperr.New(apperr.ErrInvalidInput, "service: invalid input: unknown subscription")

	// ErrUnknownCurrency is returned when the currency is not in the
	// currencies table. Mapped to 400.
	ErrUnknownCurrency = apperr.New(apperr.ErrInvalidInput, "service: invalid input: unknown currency")
)

// repo is the slice of payments persistence the service needs; *Store
// satisfies it.
type repo interface {
	GetByID(ctx context.Context, id int64) (domain.Transaction, error)
	GetForUpdate(ctx context.Context, id int64) (domain.Transaction, error)
	List(ctx context.Context, f domain.TransactionFilter, order paging.SortOrder, limit, offset int) ([]domain.Transaction, int64, error)
	Create(ctx context.Context, t domain.Transaction) (domain.Transaction, error)
	SetStatus(ctx context.Context, id int64, status domain.TransactionStatus, method, invoice string) (domain.Transaction, error)
}

// subscriptions is what a manual payment needs to know about its
// subscription; *subscription.Store satisfies it structurally.
type subscriptions interface {
	GetByID(ctx context.Context, id int64) (domain.Subscription, error)
	CurrencyExists(ctx context.Context, code string) (bool, error)
}

// transactor runs fn in one transaction; *pg.TxRunner satisfies it.
type transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// auditor records a change in the audit trail; *audit.Store satisfies it.
type auditor interface {
	Record(ctx context.Context, e domain.AuditEntry) error
}

// Service implements the payments use cases. Access is limited to the admin
// by the routes.
type Service struct {
	repo  repo
	subs  subscriptions
	tx    transactor
	audit auditor
}

// New wires a Service.
func New(r repo, subs subscriptions, tx transactor, a auditor) *Service {
	return &Service{repo: r, subs: subs, tx: tx, audit: a}
}

// Get returns one payment.
//
// Errors:
//   - ErrTransactionNotFound as its name says.
//   - any other datastore failure, propagated.
func (s *Service) Get(ctx context.Context, id int64) (domain.Transaction, error) {
	t, err := s.repo.GetByID(ctx, id)
	return t, apperr.MapStorage(err, ErrTransactionNotFound)
}

// List returns one page of payments matching f, newest payment date first
// (oldest first with paging.SortAsc).
func (s *Service) List(ctx context.Context, f domain.TransactionFilter, p paging.Params) ([]domain.Transaction, int64, error) {
	log := logger.Op(ctx, "transaction.Service.List")
	p.Normalize()
	items, total, err := s.repo.List(ctx, f, p.Order, p.Limit, p.Offset)
	if err == nil {
		log.Debug("done", "count", len(items), "total", total, "limit", p.Limit, "offset", p.Offset)
	}
	return items, total, err
}

// All returns every payment matching f, by payment date (newest first, or
// oldest first with paging.SortAsc), page by page, for an export.
func (s *Service) All(ctx context.Context, f domain.TransactionFilter, order paging.SortOrder) ([]domain.Transaction, error) {
	var out []domain.Transaction
	for p := (paging.Params{Limit: paging.MaxLimit, Order: order}); ; p.Offset += p.Limit {
		items, total, err := s.List(ctx, f, p)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if len(items) == 0 || int64(len(out)) >= total {
			return out, nil
		}
	}
}

// CreateInput is a payment the admin enters by hand.
type CreateInput struct {
	SubscriptionID int64
	Amount         string
	// Currency of Amount; the subscription's currency when empty.
	Currency      string
	PaymentDate   *time.Time
	PaymentMethod string
	InvoiceNumber string
}

// amountRe is a non-negative amount that fits NUMERIC(14,2).
var amountRe = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,2})?$`)

// Create stores a payment entered by actor; it is paid at once. Payments of
// cancelled and expired subscriptions may be entered too (history).
//
// Errors:
//   - apperr.ErrInvalidInput for a missing or malformed field.
//   - ErrUnknownSubscription, ErrUnknownCurrency as their names say.
//   - ErrDuplicateDate when the subscription already has a payment that day.
//   - any other datastore failure, propagated.
func (s *Service) Create(ctx context.Context, actor domain.User, in CreateInput) (domain.Transaction, error) {
	log := logger.Op(ctx, "transaction.Service.Create")
	in.Amount = strings.TrimSpace(in.Amount)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	switch {
	case in.SubscriptionID <= 0:
		return domain.Transaction{}, fmt.Errorf("%w: subscription_id is required", apperr.ErrInvalidInput)
	case in.PaymentDate == nil:
		return domain.Transaction{}, fmt.Errorf("%w: payment_date is required", apperr.ErrInvalidInput)
	case !amountRe.MatchString(in.Amount):
		return domain.Transaction{}, fmt.Errorf("%w: amount must be a non-negative amount with at most 2 decimals, like 1500.00", apperr.ErrInvalidInput)
	}

	sub, err := s.subs.GetByID(ctx, in.SubscriptionID)
	if err != nil {
		return domain.Transaction{}, apperr.MapStorage(err, ErrUnknownSubscription)
	}
	if in.Currency == "" {
		in.Currency = sub.Currency
	} else if ok, err := s.subs.CurrencyExists(ctx, in.Currency); err != nil {
		return domain.Transaction{}, err
	} else if !ok {
		return domain.Transaction{}, ErrUnknownCurrency
	}

	var out domain.Transaction
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		out, err = s.repo.Create(ctx, domain.Transaction{
			SubscriptionID: in.SubscriptionID,
			Amount:         in.Amount,
			Currency:       in.Currency,
			PaymentDate:    *in.PaymentDate,
			PaymentMethod:  strings.TrimSpace(in.PaymentMethod),
			InvoiceNumber:  strings.TrimSpace(in.InvoiceNumber),
		})
		if err != nil {
			return err
		}
		return s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, "transaction.create", "transaction", out.ID, map[string]any{
			"subscription_id": out.SubscriptionID, "amount": out.Amount, "currency": out.Currency,
			"payment_date": out.PaymentDate.Format(time.DateOnly), "status": string(out.Status),
		}))
	})
	switch {
	case errors.Is(err, storage.ErrAlreadyExists):
		return domain.Transaction{}, ErrDuplicateDate
	case errors.Is(err, storage.ErrNotFound):
		// The subscription was deleted, or the currency removed, meanwhile.
		return domain.Transaction{}, fmt.Errorf("%w: unknown subscription or currency", apperr.ErrInvalidInput)
	case err != nil:
		return domain.Transaction{}, err
	}
	log.Info("transaction created", "id", out.ID, "subscription_id", out.SubscriptionID, "by", actor.ID)
	return out, nil
}

// Pay marks a pending or failed payment paid. A non-empty method or invoice
// number is stored with it.
//
// Errors:
//   - ErrTransactionNotFound as its name says.
//   - ErrBadTransition when it is already paid.
//   - any other datastore failure, propagated.
func (s *Service) Pay(ctx context.Context, actor domain.User, id int64, method, invoice string) (domain.Transaction, error) {
	return s.move(ctx, actor, id, domain.TxPaid, strings.TrimSpace(method), strings.TrimSpace(invoice))
}

// Fail marks a pending payment failed.
//
// Errors:
//   - ErrTransactionNotFound as its name says.
//   - ErrBadTransition when it is not pending.
//   - any other datastore failure, propagated.
func (s *Service) Fail(ctx context.Context, actor domain.User, id int64) (domain.Transaction, error) {
	return s.move(ctx, actor, id, domain.TxFailed, "", "")
}

func (s *Service) move(ctx context.Context, actor domain.User, id int64, to domain.TransactionStatus, method, invoice string) (domain.Transaction, error) {
	log := logger.Op(ctx, "transaction.Service.move")
	var out domain.Transaction
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		cur, err := s.repo.GetForUpdate(ctx, id)
		if err != nil {
			return apperr.MapStorage(err, ErrTransactionNotFound)
		}
		if !cur.Status.CanBecome(to) {
			return fmt.Errorf("%w: %s → %s", ErrBadTransition, cur.Status, to)
		}
		if out, err = s.repo.SetStatus(ctx, id, to, method, invoice); err != nil {
			return apperr.MapStorage(err, ErrTransactionNotFound)
		}
		details := map[string]any{"status": string(to), "from": string(cur.Status)}
		if method != "" {
			details["payment_method"] = method
		}
		if invoice != "" {
			details["invoice_number"] = invoice
		}
		action := "transaction.pay"
		if to == domain.TxFailed {
			action = "transaction.fail"
		}
		return s.audit.Record(ctx, domain.NewAuditEntry(actor.ID, action, "transaction", id, details))
	})
	if err != nil {
		return domain.Transaction{}, err
	}
	log.Info("transaction status changed", "id", id, "status", to, "by", actor.ID)
	return out, nil
}
