// Package xlsx writes report workbooks (Excel .xlsx) and sends them as a
// download. Each domain builds its own sheets from its own data; this
// package only owns the format: a bold frozen header with an autofilter,
// columns wide enough for their content, amounts as numbers with two
// decimals and days as dates, so the file can be summed and sorted in Excel
// as is.
package xlsx

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"subtrack/internal/httpx"
)

// ContentType is the media type of an .xlsx file.
const ContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// Money is an amount as the API carries it ("1500.00"). It is written as a
// number formatted "# ##0.00": Excel stores numbers as doubles anyway, and a
// text cell could not be summed.
type Money string

// Day is a calendar day, written as a date cell (dd.mm.yyyy).
type Day time.Time

// DayPtr converts a nullable day; nil becomes an empty cell.
func DayPtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return Day(*t)
}

// Sheet is one tab of a workbook. Rows hold string, int, int64, float64,
// bool, Money, Day, time.Time (written as date and time) or nil (an empty
// cell); a pointer to one of these is followed.
type Sheet struct {
	Name   string
	Header []string
	Rows   [][]any
}

// Write builds a workbook from sheets, in order, and writes it to buf.
func Write(buf *bytes.Buffer, sheets ...Sheet) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	head, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"DDE5F0"}},
	})
	if err != nil {
		return err
	}
	moneyFmt := "#,##0.00"
	moneyStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &moneyFmt})
	if err != nil {
		return err
	}
	dayFmt := "dd.mm.yyyy"
	dayStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &dayFmt})
	if err != nil {
		return err
	}
	timeFmt := "dd.mm.yyyy hh:mm"
	timeStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &timeFmt})
	if err != nil {
		return err
	}

	for i, sh := range sheets {
		name := sh.Name
		if i == 0 {
			if err := f.SetSheetName("Sheet1", name); err != nil {
				return err
			}
		} else if _, err := f.NewSheet(name); err != nil {
			return err
		}
		widths := make([]int, len(sh.Header))
		for c, h := range sh.Header {
			cell, _ := excelize.CoordinatesToCellName(c+1, 1)
			if err := f.SetCellValue(name, cell, h); err != nil {
				return err
			}
			widths[c] = utf8.RuneCountInString(h)
		}
		if len(sh.Header) > 0 {
			last, _ := excelize.CoordinatesToCellName(len(sh.Header), 1)
			if err := f.SetCellStyle(name, "A1", last, head); err != nil {
				return err
			}
			if err := f.SetPanes(name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
				return err
			}
			end, _ := excelize.CoordinatesToCellName(len(sh.Header), max(len(sh.Rows)+1, 2))
			if err := f.AutoFilter(name, "A1:"+end, nil); err != nil {
				return err
			}
		}
		for r, row := range sh.Rows {
			for c, v := range row {
				cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
				val, style, text := cellValue(v)
				if val == nil {
					continue
				}
				if err := f.SetCellValue(name, cell, val); err != nil {
					return err
				}
				switch style {
				case "money":
					err = f.SetCellStyle(name, cell, cell, moneyStyle)
				case "day":
					err = f.SetCellStyle(name, cell, cell, dayStyle)
				case "time":
					err = f.SetCellStyle(name, cell, cell, timeStyle)
				}
				if err != nil {
					return err
				}
				if c < len(widths) {
					widths[c] = max(widths[c], utf8.RuneCountInString(text))
				}
			}
		}
		for c, w := range widths {
			col, _ := excelize.ColumnNumberToName(c + 1)
			if err := f.SetColWidth(name, col, col, float64(min(max(w, 6), 80))+2); err != nil {
				return err
			}
		}
	}
	return f.Write(buf)
}

// cellValue turns a row value into what excelize writes, the style to apply
// and its text (for the column width). A nil value is an empty cell.
func cellValue(v any) (val any, style, text string) {
	switch x := v.(type) {
	case nil:
		return nil, "", ""
	case *string:
		if x == nil {
			return nil, "", ""
		}
		return cellValue(*x)
	case *int:
		if x == nil {
			return nil, "", ""
		}
		return cellValue(*x)
	case *int64:
		if x == nil {
			return nil, "", ""
		}
		return cellValue(*x)
	case *float64:
		if x == nil {
			return nil, "", ""
		}
		return cellValue(*x)
	case *Money:
		if x == nil {
			return nil, "", ""
		}
		return cellValue(*x)
	case *time.Time:
		if x == nil {
			return nil, "", ""
		}
		return cellValue(*x)
	case Money:
		n, err := strconv.ParseFloat(string(x), 64)
		if err != nil {
			return string(x), "", string(x)
		}
		return n, "money", string(x) + "  "
	case Day:
		return time.Time(x), "day", "00.00.0000"
	case time.Time:
		return x, "time", "00.00.0000 00:00"
	case string:
		return x, "", x
	default:
		return x, "", fmt.Sprint(x)
	}
}

// Respond writes sheets as a download named filename. The workbook is built
// in memory first, so a failure is still an ordinary JSON 500.
func Respond(ctx context.Context, w http.ResponseWriter, filename string, sheets ...Sheet) {
	var buf bytes.Buffer
	if err := Write(&buf, sheets...); err != nil {
		httpx.Internal(ctx, w, fmt.Errorf("xlsx: %w", err))
		return
	}
	w.Header().Set("Content-Type", ContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// Filename is "subtrack-<what>-<YYYY-MM-DD>.xlsx" for today in loc.
func Filename(what string, now time.Time, loc *time.Location) string {
	return "subtrack-" + what + "-" + now.In(loc).Format(time.DateOnly) + ".xlsx"
}
