package currency

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"subtrack/internal/apperr"
	"subtrack/internal/domain"
	"subtrack/internal/storage"
	"subtrack/internal/testkit"
)

// fakeRepo keeps currencies in memory and behaves like the schema: a
// duplicate code is a unique violation, a currency in inUse cannot be
// deleted.
type fakeRepo struct {
	mu    sync.Mutex
	items map[string]domain.Currency
	inUse map[string]bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		items: map[string]domain.Currency{
			"RUB": {Code: "RUB", Name: "Российский рубль", RateToRUB: "1"},
			"USD": {Code: "USD", Name: "Доллар США", RateToRUB: "92.5"},
			"EUR": {Code: "EUR", Name: "Евро", RateToRUB: "100"},
		},
		inUse: map[string]bool{"USD": true},
	}
}

func (f *fakeRepo) List(context.Context) ([]domain.Currency, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.Currency, 0, len(f.items))
	for _, c := range f.items {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Code == "RUB") != (out[j].Code == "RUB") {
			return out[i].Code == "RUB"
		}
		return out[i].Code < out[j].Code
	})
	return out, nil
}

func (f *fakeRepo) GetByCode(_ context.Context, code string) (domain.Currency, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.items[code]
	if !ok {
		return domain.Currency{}, storage.ErrNotFound
	}
	return c, nil
}

func (f *fakeRepo) GetForUpdate(ctx context.Context, code string) (domain.Currency, error) {
	return f.GetByCode(ctx, code)
}

func (f *fakeRepo) Create(_ context.Context, c domain.Currency, actorID int64) (domain.Currency, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.items[c.Code]; ok {
		return domain.Currency{}, &storage.ConstraintError{Constraint: "currencies_pkey"}
	}
	c.UpdatedAt, c.UpdatedBy = time.Now(), &actorID
	f.items[c.Code] = c
	return c, nil
}

func (f *fakeRepo) Update(_ context.Context, c domain.Currency, actorID int64) (domain.Currency, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.items[c.Code]; !ok {
		return domain.Currency{}, storage.ErrNotFound
	}
	c.UpdatedAt, c.UpdatedBy = time.Now(), &actorID
	f.items[c.Code] = c
	return c, nil
}

func (f *fakeRepo) Delete(_ context.Context, code string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.items[code]; !ok {
		return storage.ErrNotFound
	}
	if f.inUse[code] {
		return errInUse
	}
	delete(f.items, code)
	return nil
}

var admin = domain.User{ID: 1, Role: domain.RoleAdmin}

func newService() (*Service, *fakeRepo, *testkit.Audit) {
	repo, trail := newFakeRepo(), &testkit.Audit{}
	return New(repo, &testkit.Tx{}, trail), repo, trail
}

func ptr(s string) *string { return &s }

func TestListPutsRUBFirst(t *testing.T) {
	svc, _, _ := newService()
	items, err := svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, c := range items {
		codes = append(codes, c.Code)
	}
	if len(codes) != 3 || codes[0] != "RUB" || codes[1] != "EUR" || codes[2] != "USD" {
		t.Errorf("codes = %v, want [RUB EUR USD]", codes)
	}
}

func TestGet(t *testing.T) {
	svc, _, _ := newService()
	c, err := svc.Get(context.Background(), " usd ")
	if err != nil || c.Code != "USD" {
		t.Errorf("Get(usd) = %+v, %v", c, err)
	}
	for _, code := range []string{"GBP", "US", "USDT", ""} {
		if _, err := svc.Get(context.Background(), code); !errors.Is(err, ErrCurrencyNotFound) {
			t.Errorf("Get(%q) err = %v, want ErrCurrencyNotFound", code, err)
		}
	}
}

func TestCreate(t *testing.T) {
	svc, repo, trail := newService()
	out, err := svc.Create(context.Background(), admin, domain.Currency{Code: " cny ", Name: " Юань ", RateToRUB: "12.654321"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Code != "CNY" || out.Name != "Юань" || out.RateToRUB != "12.654321" || repo.items["CNY"].Code != "CNY" {
		t.Errorf("Create = %+v", out)
	}
	e := trail.Last()
	if e.Action != "currency.create" || e.EntityType != "currency" || e.EntityID != nil || *e.ActorID != 1 ||
		e.Details["code"] != "CNY" || e.Details["rate_to_rub"] != "12.654321" {
		t.Errorf("audit = %+v", e)
	}
}

func TestCreateRejects(t *testing.T) {
	cases := []struct {
		name string
		in   domain.Currency
		want error
	}{
		{"no code", domain.Currency{Name: "X", RateToRUB: "1"}, apperr.ErrInvalidInput},
		{"no name", domain.Currency{Code: "GBP", RateToRUB: "1"}, apperr.ErrInvalidInput},
		{"no rate", domain.Currency{Code: "GBP", Name: "Фунт"}, apperr.ErrInvalidInput},
		{"code with digits", domain.Currency{Code: "US1", Name: "X", RateToRUB: "1"}, apperr.ErrInvalidInput},
		{"code too long", domain.Currency{Code: "USDT", Name: "X", RateToRUB: "1"}, apperr.ErrInvalidInput},
		{"cyrillic code", domain.Currency{Code: "РУБ", Name: "X", RateToRUB: "1"}, apperr.ErrInvalidInput},
		{"zero rate", domain.Currency{Code: "GBP", Name: "Фунт", RateToRUB: "0.000000"}, apperr.ErrInvalidInput},
		{"negative rate", domain.Currency{Code: "GBP", Name: "Фунт", RateToRUB: "-1"}, apperr.ErrInvalidInput},
		{"7 decimals", domain.Currency{Code: "GBP", Name: "Фунт", RateToRUB: "1.1234567"}, apperr.ErrInvalidInput},
		{"exponent", domain.Currency{Code: "GBP", Name: "Фунт", RateToRUB: "1e2"}, apperr.ErrInvalidInput},
		{"taken", domain.Currency{Code: "usd", Name: "Доллар", RateToRUB: "90"}, ErrCodeTaken},
		{"RUB again", domain.Currency{Code: "RUB", Name: "Рубль", RateToRUB: "1"}, ErrCodeTaken},
	}
	for _, c := range cases {
		svc, _, trail := newService()
		if _, err := svc.Create(context.Background(), admin, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
		if len(trail.Entries) != 0 {
			t.Errorf("%s: a rejected create was audited", c.name)
		}
	}
}

func TestUpdateRate(t *testing.T) {
	svc, repo, trail := newService()
	out, err := svc.Update(context.Background(), admin, "usd", domain.CurrencyPatch{RateToRUB: ptr(" 93.1 ")})
	if err != nil {
		t.Fatal(err)
	}
	if out.RateToRUB != "93.1" || out.Name != "Доллар США" || repo.items["USD"].RateToRUB != "93.1" {
		t.Errorf("Update = %+v", out)
	}
	e := trail.Last()
	if e.Action != "currency.update" || e.Details["code"] != "USD" ||
		e.Details["rate_to_rub"] != "93.1" || e.Details["old_rate_to_rub"] != "92.5" {
		t.Errorf("audit = %+v", e)
	}
	if _, ok := e.Details["name"]; ok {
		t.Errorf("audit names a field the patch did not change: %+v", e.Details)
	}
}

func TestUpdateRUB(t *testing.T) {
	svc, repo, trail := newService()
	out, err := svc.Update(context.Background(), admin, "RUB", domain.CurrencyPatch{Name: ptr("Рубль")})
	if err != nil || out.Name != "Рубль" || out.RateToRUB != "1" {
		t.Errorf("renaming RUB = %+v, %v", out, err)
	}
	if _, ok := trail.Last().Details["rate_to_rub"]; ok {
		t.Errorf("rename audited a rate: %+v", trail.Last().Details)
	}
	for _, rate := range []string{"2", "1"} {
		if _, err := svc.Update(context.Background(), admin, "rub", domain.CurrencyPatch{RateToRUB: ptr(rate)}); !errors.Is(err, ErrBaseCurrency) {
			t.Errorf("RUB rate %s: err = %v, want ErrBaseCurrency", rate, err)
		}
	}
	if repo.items["RUB"].RateToRUB != "1" {
		t.Errorf("RUB rate = %s", repo.items["RUB"].RateToRUB)
	}
}

func TestUpdateRejects(t *testing.T) {
	cases := []struct {
		name string
		code string
		p    domain.CurrencyPatch
		want error
	}{
		{"empty patch", "USD", domain.CurrencyPatch{}, apperr.ErrInvalidInput},
		{"blank name", "USD", domain.CurrencyPatch{Name: ptr("  ")}, apperr.ErrInvalidInput},
		{"zero rate", "USD", domain.CurrencyPatch{RateToRUB: ptr("0")}, apperr.ErrInvalidInput},
		{"bad rate", "USD", domain.CurrencyPatch{RateToRUB: ptr("abc")}, apperr.ErrInvalidInput},
		{"unknown", "GBP", domain.CurrencyPatch{RateToRUB: ptr("110")}, ErrCurrencyNotFound},
		{"bad code", "US", domain.CurrencyPatch{RateToRUB: ptr("110")}, ErrCurrencyNotFound},
	}
	for _, c := range cases {
		svc, repo, trail := newService()
		if _, err := svc.Update(context.Background(), admin, c.code, c.p); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
		if len(trail.Entries) != 0 || repo.items["USD"].RateToRUB != "92.5" {
			t.Errorf("%s: a rejected update changed something", c.name)
		}
	}
}

func TestDelete(t *testing.T) {
	svc, repo, trail := newService()
	if err := svc.Delete(context.Background(), admin, "eur"); err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.items["EUR"]; ok {
		t.Error("EUR is still there")
	}
	e := trail.Last()
	if e.Action != "currency.delete" || e.Details["code"] != "EUR" || e.Details["name"] != "Евро" {
		t.Errorf("audit = %+v", e)
	}

	for code, want := range map[string]error{
		"USD": ErrCurrencyInUse,
		"RUB": ErrBaseCurrency,
		"rub": ErrBaseCurrency,
		"EUR": ErrCurrencyNotFound, // gone already
		"X":   ErrCurrencyNotFound,
	} {
		if err := svc.Delete(context.Background(), admin, code); !errors.Is(err, want) {
			t.Errorf("Delete(%s) err = %v, want %v", code, err, want)
		}
	}
	if len(trail.Entries) != 1 {
		t.Errorf("audit entries = %v, want only the first delete", trail.Actions())
	}
}

func TestWriteFailsWithAudit(t *testing.T) {
	repo := newFakeRepo()
	svc := New(repo, &testkit.Tx{}, &testkit.Audit{Err: errors.New("audit down")})
	if _, err := svc.Create(context.Background(), admin, domain.Currency{Code: "GBP", Name: "Фунт", RateToRUB: "110"}); err == nil {
		t.Error("want the audit failure to fail the create")
	}
	if _, err := svc.Update(context.Background(), admin, "USD", domain.CurrencyPatch{RateToRUB: ptr("90")}); err == nil {
		t.Error("want the audit failure to fail the update")
	}
	if err := svc.Delete(context.Background(), admin, "EUR"); err == nil {
		t.Error("want the audit failure to fail the delete")
	}
}
