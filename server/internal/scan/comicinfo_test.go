package scan

import (
	"reflect"
	"strings"
	"testing"
)

const sampleComicInfo = `<?xml version="1.0" encoding="utf-8"?>
<ComicInfo>
  <Title>  第5巻  </Title>
  <Series>夜更けの測量士</Series>
  <Number>5</Number>
  <Count>7</Count>
  <Summary>静かな町の測量士の物語。</Summary>
  <Year>2021</Year>
  <Month>0</Month>
  <Day>0</Day>
  <Writer>A, B</Writer>
  <Penciller>C</Penciller>
  <Inker>B</Inker>
  <Colorist>D</Colorist>
  <Letterer>E</Letterer>
  <CoverArtist>F</CoverArtist>
  <Editor>G</Editor>
  <Translator>H</Translator>
  <Publisher>出版社</Publisher>
  <Genre>Drama, Slice of Life, Drama</Genre>
  <Tags>a, , b</Tags>
  <LanguageISO>ja</LanguageISO>
  <Manga>YesAndRightToLeft</Manga>
  <GTIN>978-4-06-123456-7</GTIN>
  <Pages><Page Image="0" Type="FrontCover"/></Pages>
  <Unknown>ignored</Unknown>
</ComicInfo>`

func TestParseComicInfoSample(t *testing.T) {
	c, err := ParseComicInfo(strings.NewReader(sampleComicInfo))
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string][2]any{
		"Title":       {c.Title, "第5巻"},
		"Series":      {c.Series, "夜更けの測量士"},
		"Number":      {c.Number, "5"},
		"Count":       {c.Count, 7},
		"Summary":     {c.Summary, "静かな町の測量士の物語。"},
		"Year":        {c.Year, 2021},
		"Month":       {c.Month, 0},
		"Day":         {c.Day, 0},
		"Publisher":   {c.Publisher, "出版社"},
		"LanguageISO": {c.LanguageISO, "ja"},
		"Manga":       {c.Manga, "YesAndRightToLeft"},
		"GTIN":        {c.GTIN, "978-4-06-123456-7"},
	}
	for name, v := range checks {
		if !reflect.DeepEqual(v[0], v[1]) {
			t.Errorf("%s = %v, want %v", name, v[0], v[1])
		}
	}

	wantAuthors := []Author{
		{"A", "writer"},
		{"B", "writer"},
		{"C", "penciller"},
		{"B", "penciller"},
		{"D", "colorist"},
		{"F", "cover"},
		{"G", "editor"},
		{"H", "translator"},
	}
	if got := c.Authors(); !reflect.DeepEqual(got, wantAuthors) {
		t.Errorf("Authors() = %v, want %v", got, wantAuthors)
	}
	if got := c.Direction(); got != "rtl" {
		t.Errorf("Direction() = %q", got)
	}
	if got := c.ReleaseDate(); got != "2021-01-01" {
		t.Errorf("ReleaseDate() = %q", got)
	}
	if got := c.ISBN(); got != "9784061234567" {
		t.Errorf("ISBN() = %q", got)
	}
	if got := c.GenreList(); !reflect.DeepEqual(got, []string{"Drama", "Slice of Life"}) {
		t.Errorf("GenreList() = %q", got)
	}
	if got := c.TagList(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("TagList() = %q", got)
	}
	if v, ok := c.NumberValue(); !ok || v != 5 {
		t.Errorf("NumberValue() = %v, %v", v, ok)
	}
}

func TestParseComicInfoErrors(t *testing.T) {
	tests := map[string]string{
		"malformed":  `<ComicInfo><Title>x</ComicInfo>`,
		"wrong root": `<Other><Title>x</Title></Other>`,
		"empty":      ``,
		"oversized":  `<ComicInfo><Summary>` + strings.Repeat("a", 1<<20) + `</Summary></ComicInfo>`,
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseComicInfo(strings.NewReader(in)); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestParseComicInfoTolerant(t *testing.T) {
	in := `<ComicInfo><Title> x </Title><Year>abc</Year><Month></Month><Day>
	 3 </Day><Count>n/a</Count></ComicInfo>`
	c, err := ParseComicInfo(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "x" || c.Year != 0 || c.Month != 0 || c.Day != 3 || c.Count != 0 {
		t.Fatalf("got %+v", c)
	}
	if c.ReleaseDate() != "" {
		t.Errorf("ReleaseDate() = %q, want empty when Year is 0", c.ReleaseDate())
	}
}

func TestComicInfoMethods(t *testing.T) {
	t.Run("direction", func(t *testing.T) {
		for in, want := range map[string]string{
			"YesAndRightToLeft": "rtl",
			"yesandrighttoleft": "rtl",
			"Yes":               "",
			"":                  "",
		} {
			if got := (ComicInfo{Manga: in}).Direction(); got != want {
				t.Errorf("Manga %q: Direction() = %q, want %q", in, got, want)
			}
		}
	})
	t.Run("release date", func(t *testing.T) {
		tests := []struct {
			c    ComicInfo
			want string
		}{
			{ComicInfo{Year: 2021, Month: 3, Day: 15}, "2021-03-15"},
			{ComicInfo{Year: 2021}, "2021-01-01"},
			{ComicInfo{Year: 2021, Month: 13, Day: 40}, "2021-01-01"},
			{ComicInfo{Year: 2021, Month: 2, Day: 30}, "2021-02-01"},
			{ComicInfo{Year: 2020, Month: 2, Day: 29}, "2020-02-29"},
			{ComicInfo{Month: 5, Day: 5}, ""},
		}
		for _, tt := range tests {
			if got := tt.c.ReleaseDate(); got != tt.want {
				t.Errorf("%+v: ReleaseDate() = %q, want %q", tt.c, got, tt.want)
			}
		}
	})
	t.Run("number value", func(t *testing.T) {
		tests := []struct {
			in   string
			want float64
			ok   bool
		}{
			{"5", 5, true},
			{" ５.5 ", 5.5, true},
			{"", 0, false},
			{"NaN", 0, false},
			{"Inf", 0, false},
			{"第5巻", 0, false},
		}
		for _, tt := range tests {
			got, ok := ComicInfo{Number: tt.in}.NumberValue()
			if got != tt.want || ok != tt.ok {
				t.Errorf("Number %q: got (%v, %v), want (%v, %v)", tt.in, got, ok, tt.want, tt.ok)
			}
		}
	})
	t.Run("isbn", func(t *testing.T) {
		tests := map[string]string{
			"978-4-06-123456-7": "9784061234567",
			"4-06-123456-X":     "406123456X",
			"4 06 123456 x":     "406123456X",
			"123456789":         "",
			"97840612345678":    "",
			"978406123456A":     "",
			"":                  "",
		}
		for in, want := range tests {
			if got := (ComicInfo{GTIN: in}).ISBN(); got != want {
				t.Errorf("GTIN %q: ISBN() = %q, want %q", in, got, want)
			}
		}
	})
	t.Run("authors dedupe and empties", func(t *testing.T) {
		c := ComicInfo{Writer: " X ,, X , ", Inker: "X", Penciller: "X", Letterer: "Z"}
		want := []Author{{"X", "writer"}, {"X", "penciller"}}
		if got := c.Authors(); !reflect.DeepEqual(got, want) {
			t.Errorf("Authors() = %v, want %v", got, want)
		}
		if got := (ComicInfo{}).Authors(); len(got) != 0 {
			t.Errorf("Authors() on empty = %v", got)
		}
	})
}
