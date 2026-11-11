package gherkin

import (
	"testing"

	"github.com/cucumber/godog"
	messages "github.com/cucumber/messages/go/v34"
)

func TestDecodeTable(t *testing.T) {
	type target struct {
		Name    string   `table:"name"`
		Limit   *int64   `table:"limit"`
		Allowed *bool    `table:"allowed"`
		Windows []string `table:"window"`
	}

	table := func(rows ...[]string) *godog.Table {
		tbl := &godog.Table{}
		for _, row := range rows {
			cells := make([]*messages.PickleTableCell, len(row))
			for i, value := range row {
				cells[i] = &messages.PickleTableCell{Value: value}
			}
			tbl.Rows = append(tbl.Rows, &messages.PickleTableRow{Cells: cells})
		}
		return tbl
	}

	t.Run("fills fields", func(t *testing.T) {
		var got target
		err := DecodeTable(table(
			[]string{"name", "dani"},
			[]string{"limit", "15"},
			[]string{"allowed", "true"},
			[]string{"window", "mon 00:00-23:59"},
			[]string{"window", "tue 10:00-18:00"},
		), &got)
		if err != nil {
			t.Fatalf("DecodeTable() error = %v", err)
		}
		if got.Name != "dani" || got.Limit == nil || *got.Limit != 15 ||
			got.Allowed == nil || !*got.Allowed {
			t.Errorf("decoded = %+v, want dani/15/true", got)
		}
		if len(got.Windows) != 2 || got.Windows[1] != "tue 10:00-18:00" {
			t.Errorf("windows = %v, want two windows", got.Windows)
		}
	})

	t.Run("rejects unknown keys", func(t *testing.T) {
		var got target
		if err := DecodeTable(table([]string{"nope", "x"}), &got); err == nil {
			t.Fatal("DecodeTable() error = nil, want unknown-key error")
		}
	})

	t.Run("rejects duplicate scalars", func(t *testing.T) {
		var got target
		err := DecodeTable(table(
			[]string{"name", "a"},
			[]string{"name", "b"},
		), &got)
		if err == nil {
			t.Fatal("DecodeTable() error = nil, want duplicate-key error")
		}
	})

	t.Run("rejects malformed rows", func(t *testing.T) {
		var got target
		if err := DecodeTable(table([]string{"name"}), &got); err == nil {
			t.Fatal("DecodeTable() error = nil, want cell-count error")
		}
	})
}
