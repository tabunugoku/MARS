// Package natsort compares names the way people number files: "p2" before
// "p10", case-insensitively, with full-width digits read as digits.
package natsort

import (
	"strings"
	"unicode"
)

// Less reports whether a sorts before b. Runs of digits compare by value
// (then by length, so "01" sorts before "1" consistently); other runes
// compare case-insensitively. Ties fall back to plain byte order so the
// result is a total order.
func Less(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		ca, cb := ra[i], rb[j]
		if isDigit(ca) && isDigit(cb) {
			si, sj := i, j
			for i < len(ra) && isDigit(ra[i]) {
				i++
			}
			for j < len(rb) && isDigit(rb[j]) {
				j++
			}
			na := strings.TrimLeft(digits(ra[si:i]), "0")
			nb := strings.TrimLeft(digits(rb[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			if i-si != j-sj {
				return i-si > j-sj // more leading zeros first
			}
			continue
		}
		la, lb := unicode.ToLower(ca), unicode.ToLower(cb)
		if la != lb {
			return la < lb
		}
		i++
		j++
	}
	if len(ra)-i != len(rb)-j {
		return len(ra)-i < len(rb)-j
	}
	return a < b
}

func isDigit(r rune) bool { return ('0' <= r && r <= '9') || ('０' <= r && r <= '９') }

func digits(rs []rune) string {
	b := make([]byte, len(rs))
	for k, r := range rs {
		if r >= '０' {
			r = r - '０' + '0'
		}
		b[k] = byte(r)
	}
	return string(b)
}
