// Package currency keeps the list of currencies money may be recorded in and
// their rates to roubles. Everybody signed in reads the list (the request
// form needs it); only the admin adds, edits and deletes (see the routes).
//
// Rules:
//   - the code is three Latin letters, stored upper-case;
//   - the rate is above 0 with at most 6 decimals, NUMERIC(18,6);
//   - RUB is the reporting currency: its rate stays 1 and it cannot be
//     deleted (its name may change);
//   - only a currency nothing uses can be deleted: subscriptions, requests
//     and payments keep pointing at theirs;
//   - a new rate applies at once to every report; rates have no history,
//     but the audit trail records the old and the new one.
package currency

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"subtrack/internal/apperr"
	"subtrack/internal/domain"
	"subtrack/internal/logger"
	"subtrack/internal/storage"
)

var (
	// ErrCurrencyNotFound is returned when no currency has the code. Mapped
	// to 404.
	ErrCurrencyNotFound = apperr.New(apperr.ErrNotFound, "service: currency not found")

	// ErrCodeTaken is returned when a currency with the code already exists.
	// Mapped to 409.
	ErrCodeTaken = apperr.New(apperr.ErrConflict, "service: a currency with this code already exists")

	// ErrBaseCurrency is returned when the rate of RUB is changed or RUB is
	// deleted. Mapped to 409.
	ErrBaseCurrency = apperr.New(apperr.ErrConflict, "service: RUB is the reporting currency: its rate is fixed at 1 and it cannot be deleted")

	// ErrCurrencyInUse is returned when a deleted currency is still used by
	// subscriptions, requests or payments. Mapped to 409.
	ErrCurrencyInUse = apperr.New(apperr.ErrConflict, "service: the currency is used by subscriptions, requests or payments and cannot be deleted")
)

// repo is the slice of persistence the service needs; *Store satisfies it.
type repo interface {
	List(ctx context.Context) ([]domain.Currency, error)
	GetByCode(ctx context.Context, code string) (domain.Currency, error)
	GetForUpdate(ctx context.Context, code string) (domain.Currency, error)
	Create(ctx context.Context, c domain.Currency, actorID int64) (domain.Currency, error)
	Update(ctx context.Context, c domain.Currency, actorID int64) (domain.Currency, error)
	Delete(ctx context.Context, code string) error
}

// transactor runs fn in one transaction; *pg.TxRunner satisfies it.
type transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// auditor records a change in the audit trail; *audit.Store satisfies it.
type auditor interface {
	Record(ctx context.Context, e domain.AuditEntry) error
}

// Service implements the currencies use cases. Every write runs in a
// transaction together with its audit record.
type Service struct {
	repo  repo
	tx    transactor
	audit auditor
}

// New wires a Service.
func New(r repo, tx transactor, a auditor) *Service {
	return &Service{repo: r, tx: tx, audit: a}
}

// codeRe is an ISO 4217 code, after upper-casing.
var codeRe = regexp.MustCompile(`^[A-Z]{3}$`)

// rateRe is a non-negative amount that fits NUMERIC(18,6); zero is refused
// separately.
var rateRe = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,6})?$`)

// normCode upper-cases a code from the path; ok is false when it cannot be a
// currency code at all.
func normCode(code string) (string, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	return code, codeRe.MatchString(code)
}

// checkRate validates a rate as the API carries it.
func checkRate(rate string) error {
	if !rateRe.MatchString(rate) || strings.Trim(rate, "0.") == "" {
		return fmt.Errorf("%w: rate_to_rub must be above 0 with at most 6 decimals, like 92.5", apperr.ErrInvalidInput)
	}
	return nil
}

// auditEntry is the audit record of a change to the currency code. The
// entity id column is a number, so the code goes into the details.
func auditEntry(actor domain.User, action, code string, details map[string]any) domain.AuditEntry {
	details["code"] = code
	return domain.AuditEntry{ActorID: &actor.ID, Action: action, EntityType: "currency", Details: details}
}

// List returns every currency, RUB first.
func (s *Service) List(ctx context.Context) ([]domain.Currency, error) {
	return s.repo.List(ctx)
}

// Get returns the currency code (any case).
//
// Errors:
//   - ErrCurrencyNotFound as its name says.
//   - any other datastore failure, propagated.
func (s *Service) Get(ctx context.Context, code string) (domain.Currency, error) {
	code, ok := normCode(code)
	if !ok {
		return domain.Currency{}, ErrCurrencyNotFound
	}
	c, err := s.repo.GetByCode(ctx, code)
	return c, apperr.MapStorage(err, ErrCurrencyNotFound)
}

// Create adds a currency on behalf of actor (an admin).
//
// Errors:
//   - apperr.ErrInvalidInput for a missing or malformed field.
//   - ErrCodeTaken when the code exists (RUB always does).
//   - any other datastore failure, propagated.
func (s *Service) Create(ctx context.Context, actor domain.User, c domain.Currency) (domain.Currency, error) {
	log := logger.Op(ctx, "currency.Service.Create")
	c.Code = strings.ToUpper(strings.TrimSpace(c.Code))
	c.Name = strings.TrimSpace(c.Name)
	c.RateToRUB = strings.TrimSpace(c.RateToRUB)

	var missing []string
	if c.Code == "" {
		missing = append(missing, "code")
	}
	if c.Name == "" {
		missing = append(missing, "name")
	}
	if c.RateToRUB == "" {
		missing = append(missing, "rate_to_rub")
	}
	if len(missing) > 0 {
		return domain.Currency{}, fmt.Errorf("%w: missing required fields: %s", apperr.ErrInvalidInput, strings.Join(missing, ", "))
	}
	if !codeRe.MatchString(c.Code) {
		return domain.Currency{}, fmt.Errorf("%w: code must be three Latin letters, like USD", apperr.ErrInvalidInput)
	}
	if err := checkRate(c.RateToRUB); err != nil {
		return domain.Currency{}, err
	}

	var out domain.Currency
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if out, err = s.repo.Create(ctx, c, actor.ID); err != nil {
			return err
		}
		return s.audit.Record(ctx, auditEntry(actor, "currency.create", out.Code,
			map[string]any{"name": out.Name, "rate_to_rub": out.RateToRUB}))
	})
	if errors.Is(err, storage.ErrAlreadyExists) {
		return domain.Currency{}, ErrCodeTaken
	}
	if err != nil {
		return domain.Currency{}, err
	}
	log.Info("currency created", "code", out.Code, "rate_to_rub", out.RateToRUB, "by", actor.ID)
	return out, nil
}

// Update changes the name and/or the rate of the currency code on behalf of
// actor (an admin).
//
// Errors:
//   - apperr.ErrInvalidInput for an empty patch or an invalid value.
//   - ErrCurrencyNotFound as its name says.
//   - ErrBaseCurrency when the patch carries a rate for RUB.
//   - any other datastore failure, propagated.
func (s *Service) Update(ctx context.Context, actor domain.User, code string, p domain.CurrencyPatch) (domain.Currency, error) {
	log := logger.Op(ctx, "currency.Service.Update")
	code, ok := normCode(code)
	if !ok {
		return domain.Currency{}, ErrCurrencyNotFound
	}
	if !p.HasAny() {
		return domain.Currency{}, fmt.Errorf("%w: at least one field must be provided", apperr.ErrInvalidInput)
	}
	if p.Name != nil {
		name := strings.TrimSpace(*p.Name)
		if name == "" {
			return domain.Currency{}, fmt.Errorf("%w: name must not be empty", apperr.ErrInvalidInput)
		}
		p.Name = &name
	}
	if p.RateToRUB != nil {
		if code == domain.BaseCurrency {
			return domain.Currency{}, ErrBaseCurrency
		}
		rate := strings.TrimSpace(*p.RateToRUB)
		if err := checkRate(rate); err != nil {
			return domain.Currency{}, err
		}
		p.RateToRUB = &rate
	}

	var out domain.Currency
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		cur, err := s.repo.GetForUpdate(ctx, code)
		if err != nil {
			return apperr.MapStorage(err, ErrCurrencyNotFound)
		}
		next := cur
		details := map[string]any{}
		if p.Name != nil {
			next.Name = *p.Name
			details["name"] = next.Name
		}
		if p.RateToRUB != nil {
			next.RateToRUB = *p.RateToRUB
		}
		if out, err = s.repo.Update(ctx, next, actor.ID); err != nil {
			return apperr.MapStorage(err, ErrCurrencyNotFound)
		}
		if p.RateToRUB != nil {
			details["rate_to_rub"] = out.RateToRUB
			details["old_rate_to_rub"] = cur.RateToRUB
		}
		return s.audit.Record(ctx, auditEntry(actor, "currency.update", code, details))
	})
	if err != nil {
		return domain.Currency{}, err
	}
	log.Info("currency updated", "code", code, "rate_to_rub", out.RateToRUB, "by", actor.ID)
	return out, nil
}

// Delete removes the currency code on behalf of actor (an admin), when
// nothing uses it.
//
// Errors:
//   - ErrCurrencyNotFound as its name says.
//   - ErrBaseCurrency for RUB.
//   - ErrCurrencyInUse when subscriptions, requests or payments use it.
//   - any other datastore failure, propagated.
func (s *Service) Delete(ctx context.Context, actor domain.User, code string) error {
	log := logger.Op(ctx, "currency.Service.Delete")
	code, ok := normCode(code)
	if !ok {
		return ErrCurrencyNotFound
	}
	if code == domain.BaseCurrency {
		return ErrBaseCurrency
	}
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		// Read it first: the record names what was deleted.
		cur, err := s.repo.GetForUpdate(ctx, code)
		if err != nil {
			return err
		}
		if err := s.repo.Delete(ctx, code); err != nil {
			return err
		}
		return s.audit.Record(ctx, auditEntry(actor, "currency.delete", code,
			map[string]any{"name": cur.Name, "rate_to_rub": cur.RateToRUB}))
	})
	switch {
	case errors.Is(err, errInUse):
		return ErrCurrencyInUse
	case err != nil:
		return apperr.MapStorage(err, ErrCurrencyNotFound)
	}
	log.Info("currency deleted", "code", code, "by", actor.ID)
	return nil
}
