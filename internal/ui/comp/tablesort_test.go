package comp

import (
	"math"
	"testing"
)

// A numeric column must order by the value a cell shows, not by its
// spelling. "memory descending" once put 512 kB above 4.2 MB and "9" above
// "100", because the sort compared strings.
func TestNumericColumnsSortByValue(t *testing.T) {
	tb := &Table{Cols: []Column{{Title: "memory", Sortable: true, Numeric: true}}}
	tb.SortCol, tb.SortDesc = 0, true
	tb.SetRows([]Row{
		{Key: "a", Cells: []string{"512 kB"}},
		{Key: "b", Cells: []string{"4.2 MB"}},
		{Key: "c", Cells: []string{"-"}},
		{Key: "d", Cells: []string{"9 B"}},
		{Key: "e", Cells: []string{"1.1 GB"}},
	})
	tb.applyFilter()
	got := make([]string, 0, len(tb.shown))
	for _, r := range tb.shown {
		got = append(got, r.Key)
	}
	want := []string{"e", "b", "a", "d", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("descending memory order %v, want %v", got, want)
		}
	}

	// Ascending flips the numbers but the dash still comes last: no data is
	// an absence, not a value below zero.
	tb.SortDesc = false
	tb.applyFilter()
	got = got[:0]
	for _, r := range tb.shown {
		got = append(got, r.Key)
	}
	want = []string{"d", "a", "b", "e", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ascending memory order %v, want %v", got, want)
		}
	}
}

func TestNumericValueReadsTheFormats(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"9", 9},
		{"100", 100},
		{"-6.2", -6.2},
		{"4.2 MB", 4.2e6},
		{"512 kB", 512e3},
		{"512 B", 512},
		{"1.1 GB", 1.1e9},
		{"14.3km", 14.3e3},
		{"38%", 38},
		{"  12.300", 12.3},
		{"1.2s", 1.2},
	}
	for _, c := range cases {
		if got := numericValue(c.in); got != c.want {
			t.Errorf("numericValue(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	for _, in := range []string{"-", "", "steady", "—"} {
		if got := numericValue(in); !math.IsNaN(got) {
			t.Errorf("numericValue(%q) = %v, want NaN", in, got)
		}
	}
}
