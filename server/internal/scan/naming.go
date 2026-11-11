package scan

import (
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"
)

var (
	// 第5巻, 第3話, 第2集 (decimal allowed).
	reKanjiPrefixed = regexp.MustCompile(`第\s*(\d+(?:\.\d+)?)\s*[巻話集]`)
	// vol / vol. / volume / v, at a word boundary (so "rev2" does not match).
	reVolume = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])(?:volume|vol|v)\.?\s*(\d+(?:\.\d+)?)`)
	// #7
	reHash = regexp.MustCompile(`#\s*(\d+(?:\.\d+)?)`)
	// 5巻, 3話 without 第.
	reKanjiSuffixed = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*[巻話]`)
	// Any number, for the last-number fallback.
	reNumber = regexp.MustCompile(`\d+(?:\.\d+)?`)
)

// stripExt removes a file extension. An extension must be 1-5 ASCII
// letters/digits containing at least one letter, so "Series - 3.5" keeps
// its decimal part while "Title.cbz" loses ".cbz".
func stripExt(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 {
		return name
	}
	ext := name[i+1:]
	if len(ext) == 0 || len(ext) > 5 {
		return name
	}
	hasLetter := false
	for _, r := range ext {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
		default:
			return name
		}
	}
	if !hasLetter {
		return name
	}
	return name[:i]
}

// baseName returns the last path element, accepting both / and \ separators.
func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// ParseNumber returns the volume or chapter number in a book file name.
// The extension is removed first and the name is NFKC-normalised. Patterns
// are tried in priority order and the first match wins: 第N巻/話/集, then
// vol/volume/v/#/N巻/N話, then the last number that is not a bracketed
// 4-digit year or part of a resolution such as 1920x1080.
func ParseNumber(fileName string) (float64, bool) {
	n := norm.NFKC.String(stripExt(baseName(fileName)))

	for _, re := range []*regexp.Regexp{reKanjiPrefixed, reVolume, reHash, reKanjiSuffixed} {
		if m := re.FindStringSubmatch(n); m != nil {
			return parseFloat(m[1])
		}
	}

	locs := reNumber.FindAllStringIndex(n, -1)
	for i := len(locs) - 1; i >= 0; i-- {
		s, e := locs[i][0], locs[i][1]
		if isResolution(n, s, e) || isBracketedYear(n, s, e) {
			continue
		}
		return parseFloat(n[s:e])
	}
	return 0, false
}

func parseFloat(s string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// isResolution reports whether the number at n[s:e] is one side of a
// "WxH" token such as 1920x1080.
func isResolution(n string, s, e int) bool {
	if s >= 2 && (n[s-1] == 'x' || n[s-1] == 'X') && isDigit(n[s-2]) {
		return true
	}
	if e+1 < len(n) && (n[e] == 'x' || n[e] == 'X') && isDigit(n[e+1]) {
		return true
	}
	return false
}

// isBracketedYear reports whether the number is a 4-digit year enclosed in
// () or [].
func isBracketedYear(n string, s, e int) bool {
	if e-s != 4 || strings.IndexByte(n[s:e], '.') >= 0 {
		return false
	}
	if s == 0 || e >= len(n) {
		return false
	}
	return strings.IndexByte("([", n[s-1]) >= 0 && strings.IndexByte(")]", n[e]) >= 0
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// BookTitle returns the file name without its extension, trimmed of spaces.
func BookTitle(fileName string) string {
	return strings.TrimSpace(stripExt(baseName(fileName)))
}

// SortTitle prepares a title for sorting: NFKC, lowercase, trimmed, and a
// leading English article ("the ", "a ", "an ") dropped. Japanese is kept.
func SortTitle(title string) string {
	t := strings.ToLower(strings.TrimSpace(norm.NFKC.String(title)))
	for _, article := range []string{"the ", "an ", "a "} {
		if strings.HasPrefix(t, article) {
			return strings.TrimSpace(t[len(article):])
		}
	}
	return t
}

var (
	reTrailingYear    = regexp.MustCompile(`\s*\(\d{4}\)$`)
	reTrailingBracket = regexp.MustCompile(`\s*\[[^\[\]]*\]$`)
)

// SeriesTitle derives a series name from a folder name by dropping trailing
// " (YYYY)" and " [tag]" suffixes. If nothing is left, the trimmed original
// name is returned.
func SeriesTitle(dirName string) string {
	orig := strings.TrimSpace(dirName)
	s := orig
	for {
		next := reTrailingYear.ReplaceAllString(s, "")
		next = reTrailingBracket.ReplaceAllString(next, "")
		next = strings.TrimSpace(next)
		if next == s {
			break
		}
		s = next
	}
	if s == "" {
		return orig
	}
	return s
}
