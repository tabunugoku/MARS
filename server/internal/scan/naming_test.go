package scan

import "testing"

func TestParseNumber(t *testing.T) {
	tests := []struct {
		name string
		file string
		want float64
		ok   bool
	}{
		{"kanji volume", "夜更けの測量士 第05巻.cbz", 5, true},
		{"full-width digits", "第１２巻.cbr", 12, true},
		{"v prefix", "Title v01.cbz", 1, true},
		{"vol dot", "Title Vol.3.cbz", 3, true},
		{"volume with year", "Title Volume 12 (2019).cbz", 12, true},
		{"hash", "Title #7.cb7", 7, true},
		{"zero padded fallback", "Title 012.cbz", 12, true},
		{"decimal fallback", "Title - 3.5.cbz", 3.5, true},
		{"year in parens skipped", "Title (2019) 04.cbz", 4, true},
		{"bracketed year only", "Title [2019].cbz", 0, false},
		{"resolution skipped", "Title 1920x1080 02.cbz", 2, true},
		{"rev is not v", "rev2 Title.cbz", 2, true},
		{"oneshot", "Oneshot.cbz", 0, false},
		{"suffixed kanji without 第", "5巻.cbz", 5, true},
		{"kanji chapter", "第3話.cbz", 3, true},
		{"no extension decimal kept", "Series - 3.5", 3.5, true},
		{"vol with space", "Title Vol 9.cbz", 9, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseNumber(tt.file)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("ParseNumber(%q) = (%v, %v), want (%v, %v)", tt.file, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestBookTitle(t *testing.T) {
	tests := map[string]string{
		"夜更けの測量士 第05巻.cbz": "夜更けの測量士 第05巻",
		"  Title .cbz":     "Title",
		"Series - 3.5":     "Series - 3.5",
		"dir/Title.epub":   "Title",
	}
	for in, want := range tests {
		if got := BookTitle(in); got != want {
			t.Errorf("BookTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSortTitle(t *testing.T) {
	tests := map[string]string{
		"The Long Walk": "long walk",
		"A Dog":         "dog",
		"An Apple":      "apple",
		"Theatre":       "theatre",
		"  ＡＢＣ  ":       "abc",
		"夜更けの測量士":       "夜更けの測量士",
		"the":           "the",
	}
	for in, want := range tests {
		if got := SortTitle(in); got != want {
			t.Errorf("SortTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSeriesTitle(t *testing.T) {
	tests := map[string]string{
		"Akira (1988)":            "Akira",
		"Akira [Digital]":         "Akira",
		"Akira (2019) [Digital] ": "Akira",
		"  Akira  ":               "Akira",
		"[Digital]":               "[Digital]",
		"(2019)":                  "(2019)",
		"Series (2019 Edition)":   "Series (2019 Edition)",
		"夜更けの測量士 (2020)":          "夜更けの測量士",
	}
	for in, want := range tests {
		if got := SeriesTitle(in); got != want {
			t.Errorf("SeriesTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
