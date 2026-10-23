package scheduler

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"subtrack/internal/domain"
	"subtrack/internal/storage"
	"subtrack/internal/testkit"
)

// fakeRepo is an in-memory subscriptions table with a notification_log.
// Candidates returns every live subscription: plan does the real choosing.
type fakeRepo struct {
	mu      sync.Mutex
	subs    map[int64]domain.Subscription
	holders map[int64][]int64
	log     map[string]bool
	// payments holds "<sub id>/<date>" of every payment, pending or not.
	payments map[string]bool
	nextTxID int64
	// failLock makes Lock fail for that subscription id.
	failLock int64
	// gone are ids Candidates still lists but Lock no longer finds.
	gone []int64
}

func newFakeRepo(subs ...domain.Subscription) *fakeRepo {
	f := &fakeRepo{subs: map[int64]domain.Subscription{}, holders: map[int64][]int64{}, log: map[string]bool{},
		payments: map[string]bool{}}
	for _, s := range subs {
		f.subs[s.ID] = s
	}
	return f
}

func (f *fakeRepo) Candidates(context.Context, time.Time, time.Time) ([]int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]int64, 0, len(f.subs))
	for id := range f.subs {
		ids = append(ids, id)
	}
	ids = append(ids, f.gone...)
	slices.Sort(ids)
	return ids, nil
}

func (f *fakeRepo) Lock(_ context.Context, id int64) (domain.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id == f.failLock {
		return domain.Subscription{}, errors.New("boom")
	}
	s, ok := f.subs[id]
	if !ok {
		return domain.Subscription{}, storage.ErrNotFound
	}
	return s, nil
}

func (f *fakeRepo) SetStatus(_ context.Context, id int64, status domain.SubscriptionStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.subs[id]
	s.Status = status
	f.subs[id] = s
	return nil
}

func (f *fakeRepo) CreatePending(_ context.Context, sub domain.Subscription, day time.Time) (int64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := strconv.FormatInt(sub.ID, 10) + "/" + day.Format(time.DateOnly)
	if f.payments[k] {
		return 0, false, nil
	}
	f.payments[k] = true
	f.nextTxID++
	return f.nextTxID, true, nil
}

func (f *fakeRepo) SetNextPaymentDate(_ context.Context, id int64, day time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.subs[id]
	s.NextPaymentDate = &day
	f.subs[id] = s
	return nil
}

func (f *fakeRepo) MarkSent(_ context.Context, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.log[key] {
		return false, nil
	}
	f.log[key] = true
	return true, nil
}

func (f *fakeRepo) Holders(_ context.Context, subID int64) ([]int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.holders[subID], nil
}

func (f *fakeRepo) status(id int64) domain.SubscriptionStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.subs[id].Status
}

type fakeSettings struct {
	st  domain.NotificationSettings
	err error
}

func (f *fakeSettings) Get(context.Context) (domain.NotificationSettings, error) { return f.st, f.err }

// fakeNotifier keeps what was sent; admin copies have UserID 0.
type fakeNotifier struct {
	mu   sync.Mutex
	sent []domain.Notification
}

func (f *fakeNotifier) Send(_ context.Context, n domain.Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, n)
	return nil
}

func (f *fakeNotifier) SendToAdmins(ctx context.Context, n domain.Notification) error {
	n.UserID = 0
	return f.Send(ctx, n)
}

func (f *fakeNotifier) types() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.sent))
	for _, n := range f.sent {
		out = append(out, string(n.Type)+"@"+strings.Repeat("u", int(n.UserID)))
	}
	return out
}

type fixture struct {
	repo   *fakeRepo
	st     *fakeSettings
	audit  *testkit.Audit
	notify *fakeNotifier
	sched  *Scheduler
}

// newFixture builds a scheduler whose clock reads 2026-10-23 12:00 in Moscow.
func newFixture(t *testing.T, subs ...domain.Subscription) *fixture {
	t.Helper()
	msk, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		repo:   newFakeRepo(subs...),
		st:     &fakeSettings{st: domain.NotificationSettings{ExpiringDays: 7, PaymentDays: 3}},
		audit:  &testkit.Audit{},
		notify: &fakeNotifier{},
	}
	f.sched = New(f.repo, f.st, &testkit.Tx{}, f.audit, f.notify, Config{Location: msk, Interval: time.Hour})
	f.sched.now = func() time.Time { return time.Date(2026, 10, 23, 12, 0, 0, 0, msk) }
	return f
}

func TestRunOnce(t *testing.T) {
	f := newFixture(t,
		domain.Subscription{ID: 1, Name: "Jira", Status: domain.SubActive, EndDate: dayPtr("2026-10-28")},
		domain.Subscription{ID: 2, Name: "Figma", Status: domain.SubExpiringSoon, EndDate: dayPtr("2026-10-20")},
		domain.Subscription{ID: 3, Name: "Slack", Status: domain.SubActive, NextPaymentDate: dayPtr("2026-10-25"),
			Cost: "1500.00", Currency: "RUB"},
		domain.Subscription{ID: 4, Name: "Old", Status: domain.SubCancelled, EndDate: dayPtr("2026-10-01")},
	)
	f.repo.holders[1] = []int64{2, 3}

	rep, err := f.sched.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep != (Report{Checked: 4, Changed: 2, Notified: 3}) {
		t.Errorf("report = %+v", rep)
	}
	if got := f.repo.status(1); got != domain.SubExpiringSoon {
		t.Errorf("Jira status = %s, want expiring_soon", got)
	}
	if got := f.repo.status(2); got != domain.SubExpired {
		t.Errorf("Figma status = %s, want expired", got)
	}
	if got := f.repo.status(4); got != domain.SubCancelled {
		t.Errorf("cancelled status = %s, want it left alone", got)
	}
	wantSent := []string{
		"subscription_expiring@uu", "subscription_expiring@uuu", "subscription_expiring@",
		"subscription_expired@", "payment_due@",
	}
	if got := f.notify.types(); !slices.Equal(got, wantSent) {
		t.Errorf("sent = %v, want %v", got, wantSent)
	}
	if body := f.notify.sent[4].Body; !strings.Contains(body, "25.10.2026 (через 2 дня), сумма 1500.00 RUB") {
		t.Errorf("payment body = %q", body)
	}

	if len(f.audit.Entries) != 2 {
		t.Fatalf("audit = %+v, want 2 entries", f.audit.Entries)
	}
	for _, e := range f.audit.Entries {
		if e.ActorID != nil || e.Action != "subscription.update" || e.Details["by"] != "scheduler" {
			t.Errorf("audit entry = %+v, want a system subscription.update by the scheduler", e)
		}
	}
	if e := f.audit.Entries[1]; *e.EntityID != 2 || e.Details["status"] != "expired" {
		t.Errorf("second audit entry = %+v, want subscription 2 expired", e)
	}
}

func TestRunOnceTwiceNotifiesOnce(t *testing.T) {
	f := newFixture(t,
		domain.Subscription{ID: 1, Name: "Jira", Status: domain.SubActive, EndDate: dayPtr("2026-10-28"),
			NextPaymentDate: dayPtr("2026-10-24")},
	)
	if _, err := f.sched.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := len(f.notify.sent)
	rep, err := f.sched.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Changed != 0 || rep.Notified != 0 || len(f.notify.sent) != first {
		t.Errorf("second run: report %+v, sent %d (first run sent %d), want nothing new", rep, len(f.notify.sent), first)
	}
	if len(f.audit.Entries) != 1 {
		t.Errorf("audit entries = %d, want 1", len(f.audit.Entries))
	}
}

func TestRunOnceMovedDateIsANewEvent(t *testing.T) {
	f := newFixture(t, domain.Subscription{ID: 1, Name: "Jira", Status: domain.SubActive, EndDate: dayPtr("2026-10-28")})
	if _, err := f.sched.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The admin moves the end date, still inside the window.
	s := f.repo.subs[1]
	s.EndDate = dayPtr("2026-10-29")
	f.repo.subs[1] = s
	rep, err := f.sched.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Notified != 1 || rep.Changed != 0 {
		t.Errorf("report = %+v, want one new event and no status change", rep)
	}
}

func TestRunOnceRenewed(t *testing.T) {
	f := newFixture(t, domain.Subscription{ID: 1, Name: "Jira", Status: domain.SubExpiringSoon, EndDate: dayPtr("2027-10-28")})
	rep, err := f.sched.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Changed != 1 || rep.Notified != 0 || len(f.notify.sent) != 0 {
		t.Errorf("report = %+v, sent %d, want one silent change", rep, len(f.notify.sent))
	}
	if got := f.repo.status(1); got != domain.SubActive {
		t.Errorf("status = %s, want active", got)
	}
}

func TestRunOnceReadsSettingsEveryRun(t *testing.T) {
	f := newFixture(t, domain.Subscription{ID: 1, Name: "Jira", Status: domain.SubActive, EndDate: dayPtr("2026-11-05")})
	if rep, _ := f.sched.RunOnce(context.Background()); rep.Changed != 0 {
		t.Fatalf("13 days away with a 7-day window: report = %+v, want no change", rep)
	}
	f.st.st.ExpiringDays = 14
	if rep, _ := f.sched.RunOnce(context.Background()); rep.Changed != 1 || rep.Notified != 1 {
		t.Errorf("after widening to 14 days: report = %+v, want one change and one event", rep)
	}
}

func TestRunOnceOneFailureDoesNotStopOthers(t *testing.T) {
	f := newFixture(t,
		domain.Subscription{ID: 1, Name: "A", Status: domain.SubActive, EndDate: dayPtr("2026-10-01")},
		domain.Subscription{ID: 2, Name: "B", Status: domain.SubActive, EndDate: dayPtr("2026-10-01")},
	)
	f.repo.failLock = 1
	rep, err := f.sched.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep != (Report{Checked: 2, Changed: 1, Notified: 1, Failed: 1}) {
		t.Errorf("report = %+v", rep)
	}
	if got := f.repo.status(2); got != domain.SubExpired {
		t.Errorf("B status = %s, want expired", got)
	}
}

func TestRunOnceDeletedMeanwhile(t *testing.T) {
	f := newFixture(t)
	f.repo.gone = []int64{1}
	rep, err := f.sched.RunOnce(context.Background())
	if err != nil || rep != (Report{Checked: 1}) {
		t.Errorf("rep = %+v, err = %v, want a quiet skip", rep, err)
	}
}

func TestRunOnceSettingsError(t *testing.T) {
	f := newFixture(t)
	f.st.err = errors.New("no settings")
	if _, err := f.sched.RunOnce(context.Background()); err == nil {
		t.Error("RunOnce without settings: want an error")
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	f := newFixture(t, domain.Subscription{ID: 1, Name: "A", Status: domain.SubActive, EndDate: dayPtr("2026-10-01")})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		f.sched.Run(ctx)
		close(done)
	}()
	// The first run happens right away.
	deadline := time.Now().Add(2 * time.Second)
	for f.repo.status(1) != domain.SubExpired {
		if time.Now().After(deadline) {
			t.Fatal("the first run did not happen at start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}

func TestLoadConfig(t *testing.T) {
	t.Setenv("SCHEDULER_TZ", "")
	t.Setenv("SCHEDULER_INTERVAL", "")
	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.Location.String() != "Europe/Moscow" || c.Interval != time.Hour {
		t.Errorf("defaults = %s, %s", c.Location, c.Interval)
	}

	t.Setenv("SCHEDULER_TZ", "Asia/Yekaterinburg")
	t.Setenv("SCHEDULER_INTERVAL", "15m")
	if c, err = LoadConfig(); err != nil || c.Location.String() != "Asia/Yekaterinburg" || c.Interval != 15*time.Minute {
		t.Errorf("LoadConfig = %+v, %v", c, err)
	}

	for _, bad := range [][2]string{{"Mars/Base", "1h"}, {"", "soon"}, {"", "30s"}} {
		t.Setenv("SCHEDULER_TZ", bad[0])
		t.Setenv("SCHEDULER_INTERVAL", bad[1])
		if _, err := LoadConfig(); err == nil {
			t.Errorf("LoadConfig(%q, %q): want an error", bad[0], bad[1])
		}
	}
}

func TestRunOnceCharges(t *testing.T) {
	f := newFixture(t,
		domain.Subscription{ID: 1, Name: "Jira", Status: domain.SubActive, BillingPeriod: domain.BillingMonth,
			NextPaymentDate: dayPtr("2026-09-23"), Cost: "100.00", Currency: "RUB"},
	)
	// The admin entered September by hand.
	f.repo.payments["1/2026-09-23"] = true

	rep, err := f.sched.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// September is skipped (already there), October is created and reminded
	// (it is today), and the next date moves to November.
	if rep != (Report{Checked: 1, Charged: 1, Notified: 1}) {
		t.Errorf("report = %+v", rep)
	}
	if got := f.repo.subs[1].NextPaymentDate.Format(time.DateOnly); got != "2026-11-23" {
		t.Errorf("next_payment_date = %s, want 2026-11-23", got)
	}
	if got := f.audit.Actions(); !slices.Equal(got, []string{"transaction.create", "subscription.update"}) {
		t.Errorf("audit = %v", got)
	}
	last := f.audit.Last()
	if last.ActorID != nil || last.Details["next_payment_date"] != "2026-11-23" || last.Details["by"] != "scheduler" {
		t.Errorf("last audit entry = %+v", last)
	}
	if body := f.notify.sent[0].Body; !strings.Contains(body, "23.10.2026 (сегодня)") {
		t.Errorf("payment body = %q", body)
	}

	rep, _ = f.sched.RunOnce(context.Background())
	if rep != (Report{Checked: 1}) {
		t.Errorf("second run: report = %+v, want nothing (next payment is a month away)", rep)
	}
}
