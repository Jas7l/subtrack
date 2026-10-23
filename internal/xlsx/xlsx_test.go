package xlsx

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestWriteAndRead(t *testing.T) {
	d := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	var nilStr *string
	name := "Разработка"
	var buf bytes.Buffer
	err := Write(&buf,
		Sheet{Name: "Подписки", Header: []string{"ID", "Название", "Стоимость", "Начало", "Отдел"},
			Rows: [][]any{
				{int64(1), "Jira", Money("15000.50"), Day(d), &name},
				{int64(2), "Figma", Money("bad"), DayPtr(nil), nilStr},
			}},
		Sheet{Name: "Пусто", Header: []string{"A"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.GetSheetList(); len(got) != 2 || got[0] != "Подписки" || got[1] != "Пусто" {
		t.Fatalf("sheets = %v", got)
	}
	rows, _ := f.GetRows("Подписки")
	if len(rows) != 3 || rows[0][1] != "Название" || rows[1][1] != "Jira" || rows[1][4] != "Разработка" {
		t.Fatalf("rows = %v", rows)
	}
	if rows[1][2] != "15,000.50" || rows[1][3] != "23.10.2026" {
		t.Errorf("formatted money/date = %q / %q", rows[1][2], rows[1][3])
	}
	raw, _ := f.GetCellValue("Подписки", "C2", excelize.Options{RawCellValue: true})
	if raw != "15000.5" {
		t.Errorf("money is stored as %q, want a number", raw)
	}
	if rows[2][2] != "bad" || len(rows[2]) > 3 && rows[2][3] != "" {
		t.Errorf("second row = %v", rows[2])
	}
}

func TestRespond(t *testing.T) {
	rec := httptest.NewRecorder()
	Respond(context.Background(), rec, "subtrack-x-2026-10-23.xlsx", Sheet{Name: "A", Header: []string{"x"}})
	if rec.Code != 200 || rec.Header().Get("Content-Type") != ContentType ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), `filename="subtrack-x-2026-10-23.xlsx"`) {
		t.Errorf("response %d %v", rec.Code, rec.Header())
	}
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte("PK")) {
		t.Error("body is not a zip (xlsx)")
	}
}

func TestFilename(t *testing.T) {
	msk, _ := time.LoadLocation("Europe/Moscow")
	now := time.Date(2026, 10, 23, 22, 0, 0, 0, time.UTC)
	if got := Filename("payments", now, msk); got != "subtrack-payments-2026-10-24.xlsx" {
		t.Errorf("Filename = %s", got)
	}
}
