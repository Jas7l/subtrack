package transaction

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/domain"
	"subtrack/internal/paging"
	"subtrack/internal/storage"
	"subtrack/internal/testkit"
)

// fakeRepo is an in-memory transactions table with its unique
// (subscription, date) key.
type fakeRepo struct {
	mu     sync.Mutex
	txs    map[int64]domain.Transaction
	nextID int64
}

func newFakeRepo() *fakeRepo { return &fakeRepo{txs: map[int64]domain.Transaction{}, nextID: 1} }

func (f *fakeRepo) GetByID(_ context.Context, id int64) (domain.Transaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.txs[id]
	if !ok {
		return domain.Transaction{}, storage.ErrNotFound
	}
	return t, nil
}

func (f *fakeRepo) GetForUpdate(ctx context.Context, id int64) (domain.Transaction, error) {
	return f.GetByID(ctx, id)
}

func (f *fakeRepo) List(_ context.Context, flt domain.TransactionFilter, _ paging.SortOrder, limit, offset int) ([]domain.Transaction, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Transaction
	for id := int64(1); id < f.nextID; id++ {
		t, ok := f.txs[id]
		if ok && (flt.Status == nil || t.Status == *flt.Status) {
			out = append(out, t)
		}
	}
	total := int64(len(out))
	out = out[min(offset, len(out)):]
	return out[:min(limit, len(out))], total, nil
}

func (f *fakeRepo) Create(_ context.Context, t domain.Transaction) (domain.Transaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.txs {
		if x.SubscriptionID == t.SubscriptionID && x.PaymentDate.Equal(t.PaymentDate) {
			return domain.Transaction{}, &storage.ConstraintError{Constraint: "transactions_subscription_date_key"}
		}
	}
	now := time.Now()
	t.ID, t.Status, t.PaidAt, t.CreatedAt, t.UpdatedAt = f.nextID, domain.TxPaid, &now, now, now
	f.nextID++
	f.txs[t.ID] = t
	return t, nil
}

func (f *fakeRepo) SetStatus(_ context.Context, id int64, status domain.TransactionStatus, method, invoice string) (domain.Transaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.txs[id]
	if !ok {
		return domain.Transaction{}, storage.ErrNotFound
	}
	t.Status, t.PaidAt = status, nil
	if status == domain.TxPaid {
		now := time.Now()
		t.PaidAt = &now
	}
	if method != "" {
		t.PaymentMethod = method
	}
	if invoice != "" {
		t.InvoiceNumber = invoice
	}
	f.txs[id] = t
	return t, nil
}

// pending adds a payment the way the scheduler would.
func (f *fakeRepo) pending(subID int64, day string) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, _ := time.Parse(time.DateOnly, day)
	id := f.nextID
	f.nextID++
	f.txs[id] = domain.Transaction{ID: id, SubscriptionID: subID, Amount: "100.00", Currency: "RUB",
		PaymentDate: d, Status: domain.TxPending}
	return id
}

type fakeSubs struct{}

func (fakeSubs) GetByID(_ context.Context, id int64) (domain.Subscription, error) {
	if id != 12 {
		return domain.Subscription{}, storage.ErrNotFound
	}
	return domain.Subscription{ID: 12, Name: "Jira", Currency: "USD"}, nil
}

func (fakeSubs) CurrencyExists(_ context.Context, code string) (bool, error) {
	return code == "RUB" || code == "USD", nil
}

type fixture struct {
	repo  *fakeRepo
	audit *testkit.Audit
	svc   *Service
	admin domain.User
}

func newFixture() *fixture {
	f := &fixture{repo: newFakeRepo(), audit: &testkit.Audit{}, admin: domain.User{ID: 1, Role: domain.RoleAdmin}}
	f.svc = New(f.repo, fakeSubs{}, &testkit.Tx{}, f.audit)
	return f
}

func datePtr(s string) *time.Time {
	d, _ := time.Parse(time.DateOnly, s)
	return &d
}

func TestCreate(t *testing.T) {
	f := newFixture()
	ctx := context.Background()

	out, err := f.svc.Create(ctx, f.admin, CreateInput{SubscriptionID: 12, Amount: " 1500.5 ", PaymentDate: datePtr("2026-09-01"),
		PaymentMethod: " Счёт ", InvoiceNumber: "INV-1"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != domain.TxPaid || out.Currency != "USD" || out.Amount != "1500.5" || out.PaymentMethod != "Счёт" {
		t.Errorf("Create = %+v, want paid, in the subscription's USD, trimmed", out)
	}
	e := f.audit.Last()
	if e.Action != "transaction.create" || *e.EntityID != out.ID || e.Details["payment_date"] != "2026-09-01" {
		t.Errorf("audit = %+v", e)
	}

	cases := []struct {
		name string
		in   CreateInput
		want error
	}{
		{"no subscription", CreateInput{Amount: "1", PaymentDate: datePtr("2026-09-02")}, apperr.ErrInvalidInput},
		{"no date", CreateInput{SubscriptionID: 12, Amount: "1"}, apperr.ErrInvalidInput},
		{"negative", CreateInput{SubscriptionID: 12, Amount: "-1", PaymentDate: datePtr("2026-09-02")}, apperr.ErrInvalidInput},
		{"three decimals", CreateInput{SubscriptionID: 12, Amount: "1.005", PaymentDate: datePtr("2026-09-02")}, apperr.ErrInvalidInput},
		{"unknown subscription", CreateInput{SubscriptionID: 99, Amount: "1", PaymentDate: datePtr("2026-09-02")}, ErrUnknownSubscription},
		{"unknown currency", CreateInput{SubscriptionID: 12, Amount: "1", Currency: "xxx", PaymentDate: datePtr("2026-09-02")}, ErrUnknownCurrency},
		{"same date", CreateInput{SubscriptionID: 12, Amount: "1", PaymentDate: datePtr("2026-09-01")}, ErrDuplicateDate},
	}
	for _, c := range cases {
		if _, err := f.svc.Create(ctx, f.admin, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	if n := len(f.audit.Entries); n != 1 {
		t.Errorf("audit entries = %d, want 1: failed creates record nothing", n)
	}
	if out, err := f.svc.Create(ctx, f.admin, CreateInput{SubscriptionID: 12, Amount: "1", Currency: "rub",
		PaymentDate: datePtr("2026-09-02")}); err != nil || out.Currency != "RUB" {
		t.Errorf("lower-case currency: %+v, %v", out, err)
	}
}

func TestTransitions(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		steps []string // "pay" / "fail"
		want  []error
		final domain.TransactionStatus
	}{
		{"pay", []string{"pay"}, []error{nil}, domain.TxPaid},
		{"fail then pay", []string{"fail", "pay"}, []error{nil, nil}, domain.TxPaid},
		{"pay twice", []string{"pay", "pay"}, []error{nil, ErrBadTransition}, domain.TxPaid},
		{"fail paid", []string{"pay", "fail"}, []error{nil, ErrBadTransition}, domain.TxPaid},
		{"fail twice", []string{"fail", "fail"}, []error{nil, ErrBadTransition}, domain.TxFailed},
	}
	for _, c := range cases {
		f := newFixture()
		id := f.repo.pending(12, "2026-10-23")
		for i, step := range c.steps {
			var err error
			if step == "pay" {
				_, err = f.svc.Pay(ctx, f.admin, id, "Карта", "")
			} else {
				_, err = f.svc.Fail(ctx, f.admin, id)
			}
			if !errors.Is(err, c.want[i]) {
				t.Errorf("%s step %d (%s): err = %v, want %v", c.name, i, step, err, c.want[i])
			}
		}
		got, _ := f.repo.GetByID(ctx, id)
		if got.Status != c.final || (got.Status == domain.TxPaid) != (got.PaidAt != nil) {
			t.Errorf("%s: final %s paid_at %v, want %s", c.name, got.Status, got.PaidAt, c.final)
		}
	}
}

func TestPayAudit(t *testing.T) {
	f := newFixture()
	id := f.repo.pending(12, "2026-10-23")
	out, err := f.svc.Pay(context.Background(), f.admin, id, " Карта ", "INV-9")
	if err != nil {
		t.Fatal(err)
	}
	if out.PaymentMethod != "Карта" || out.InvoiceNumber != "INV-9" || out.Amount != "100.00" {
		t.Errorf("Pay = %+v", out)
	}
	e := f.audit.Last()
	if e.Action != "transaction.pay" || e.Details["from"] != "pending" || e.Details["invoice_number"] != "INV-9" {
		t.Errorf("audit = %+v", e)
	}
	if _, err := f.svc.Pay(context.Background(), f.admin, 99, "", ""); !errors.Is(err, ErrTransactionNotFound) {
		t.Errorf("pay unknown: err = %v", err)
	}
}

func TestPayFailsWithAudit(t *testing.T) {
	f := newFixture()
	f.audit.Err = errors.New("audit down")
	id := f.repo.pending(12, "2026-10-23")
	if _, err := f.svc.Pay(context.Background(), f.admin, id, "", ""); err == nil {
		t.Error("want the audit failure to fail the payment")
	}
}

func TestList(t *testing.T) {
	f := newFixture()
	for i := range 3 {
		f.repo.pending(12, "2026-10-2"+strconv.Itoa(i))
	}
	_, _ = f.svc.Pay(context.Background(), f.admin, 1, "", "")
	st := domain.TxPending
	items, total, err := f.svc.List(context.Background(), domain.TransactionFilter{Status: &st}, paging.Params{})
	if err != nil || total != 2 || len(items) != 2 {
		t.Errorf("List pending = %d items, total %d, err %v", len(items), total, err)
	}
}
