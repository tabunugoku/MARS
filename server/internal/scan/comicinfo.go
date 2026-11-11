package scan

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

// maxComicInfoBytes caps how much of a ComicInfo.xml is read.
const maxComicInfoBytes = 1 << 20

// comicInfoXML mirrors ComicInfo with xml tags. Numbers are read as text so
// that empty or non-numeric values can be tolerated instead of failing the
// whole parse.
type comicInfoXML struct {
	XMLName     xml.Name `xml:"ComicInfo"`
	Title       string   `xml:"Title"`
	Series      string   `xml:"Series"`
	Number      string   `xml:"Number"`
	Count       string   `xml:"Count"`
	Summary     string   `xml:"Summary"`
	Year        string   `xml:"Year"`
	Month       string   `xml:"Month"`
	Day         string   `xml:"Day"`
	Writer      string   `xml:"Writer"`
	Penciller   string   `xml:"Penciller"`
	Inker       string   `xml:"Inker"`
	Colorist    string   `xml:"Colorist"`
	Letterer    string   `xml:"Letterer"`
	CoverArtist string   `xml:"CoverArtist"`
	Editor      string   `xml:"Editor"`
	Translator  string   `xml:"Translator"`
	Publisher   string   `xml:"Publisher"`
	Genre       string   `xml:"Genre"`
	Tags        string   `xml:"Tags"`
	LanguageISO string   `xml:"LanguageISO"`
	Manga       string   `xml:"Manga"`
	AgeRating   string   `xml:"AgeRating"`
	GTIN        string   `xml:"GTIN"`
}

// ParseComicInfo decodes a ComicInfo.xml document. At most 1 MiB is read;
// larger input is an error. Unknown elements are ignored, strings are
// trimmed, and Count/Year/Month/Day that are empty or non-numeric become 0.
func ParseComicInfo(r io.Reader) (ComicInfo, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxComicInfoBytes+1))
	if err != nil {
		return ComicInfo{}, err
	}
	if len(data) > maxComicInfoBytes {
		return ComicInfo{}, errors.New("comicinfo: document exceeds 1 MiB")
	}

	var x comicInfoXML
	if err := xml.Unmarshal(data, &x); err != nil {
		return ComicInfo{}, fmt.Errorf("comicinfo: %w", err)
	}

	return ComicInfo{
		Title:       strings.TrimSpace(x.Title),
		Series:      strings.TrimSpace(x.Series),
		Number:      strings.TrimSpace(x.Number),
		Count:       atoiOrZero(x.Count),
		Summary:     strings.TrimSpace(x.Summary),
		Year:        atoiOrZero(x.Year),
		Month:       atoiOrZero(x.Month),
		Day:         atoiOrZero(x.Day),
		Writer:      strings.TrimSpace(x.Writer),
		Penciller:   strings.TrimSpace(x.Penciller),
		Inker:       strings.TrimSpace(x.Inker),
		Colorist:    strings.TrimSpace(x.Colorist),
		Letterer:    strings.TrimSpace(x.Letterer),
		CoverArtist: strings.TrimSpace(x.CoverArtist),
		Editor:      strings.TrimSpace(x.Editor),
		Translator:  strings.TrimSpace(x.Translator),
		Publisher:   strings.TrimSpace(x.Publisher),
		Genre:       strings.TrimSpace(x.Genre),
		Tags:        strings.TrimSpace(x.Tags),
		LanguageISO: strings.TrimSpace(x.LanguageISO),
		Manga:       strings.TrimSpace(x.Manga),
		AgeRating:   strings.TrimSpace(x.AgeRating),
		GTIN:        strings.TrimSpace(x.GTIN),
	}, nil
}

func atoiOrZero(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

// Authors lists the contributors named in the role fields, in the order
// writer, penciller (Penciller then Inker), colorist, cover, editor,
// translator. Names are split on commas and deduplicated per (name, role).
// Letterer is not mapped and is skipped.
func (c ComicInfo) Authors() []Author {
	var out []Author
	seen := map[Author]bool{}
	add := func(field, role string) {
		for _, name := range splitList(field) {
			a := Author{Name: name, Role: role}
			if !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
		}
	}
	add(c.Writer, "writer")
	add(c.Penciller, "penciller")
	add(c.Inker, "penciller")
	add(c.Colorist, "colorist")
	add(c.CoverArtist, "cover")
	add(c.Editor, "editor")
	add(c.Translator, "translator")
	return out
}

// Direction returns "rtl" for Manga == YesAndRightToLeft (case-insensitive)
// and "" otherwise.
func (c ComicInfo) Direction() string {
	if strings.EqualFold(strings.TrimSpace(c.Manga), "YesAndRightToLeft") {
		return "rtl"
	}
	return ""
}

// ReleaseDate returns "YYYY-MM-DD" when Year is set. A Month or Day that is
// 0 or out of range defaults to 1. It returns "" when Year is not set.
func (c ComicInfo) ReleaseDate() string {
	if c.Year <= 0 {
		return ""
	}
	m := c.Month
	if m < 1 || m > 12 {
		m = 1
	}
	d := c.Day
	if d < 1 || d > daysIn(c.Year, m) {
		d = 1
	}
	return fmt.Sprintf("%04d-%02d-%02d", c.Year, m, d)
}

func daysIn(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// NumberValue parses the NFKC-normalised, trimmed Number as a float. It
// reports false when Number is empty, not a finite number.
func (c ComicInfo) NumberValue() (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(norm.NFKC.String(c.Number)), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

// GenreList splits Genre on commas into trimmed, non-empty, deduplicated values.
func (c ComicInfo) GenreList() []string { return splitList(c.Genre) }

// TagList splits Tags on commas into trimmed, non-empty, deduplicated values.
func (c ComicInfo) TagList() []string { return splitList(c.Tags) }

// ISBN returns the GTIN with hyphens and spaces removed when the result is
// 10 characters (the last may be X) or 13 digits. Otherwise it returns "".
// Full-width characters are NFKC-normalised first.
func (c ComicInfo) ISBN() string {
	s := norm.NFKC.String(c.GTIN)
	s = strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, s)
	s = strings.ToUpper(s)
	switch len(s) {
	case 13:
		if allDigits(s) {
			return s
		}
	case 10:
		if allDigits(s[:9]) && (s[9] == 'X' || isDigit(s[9])) {
			return s
		}
	}
	return ""
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

// splitList splits on commas, trims each item, drops empties and duplicates,
// and keeps the first occurrence order.
func splitList(s string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}
