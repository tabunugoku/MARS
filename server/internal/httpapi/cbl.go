package httpapi

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"

	"golang.org/x/text/unicode/norm"
)

// ComicRack CBL import. The file is parsed with encoding/xml, which never
// fetches external entities and expands only the five predefined entities
// and character references, so a DTD cannot blow up or read files. Entries
// are matched to books the caller may see through the scoped builders
// (listSeries, listBooks), so a library the caller cannot see never matches.

// maxCBL is the largest CBL file accepted.
const maxCBL = 5 << 20

// cblDoc is the part of a <ReadingList> document the import reads.
type cblDoc struct {
	XMLName xml.Name `xml:"ReadingList"`
	Name    string   `xml:"Name"`
	Books   struct {
		Items []cblBook `xml:"Book"`
	} `xml:"Books"`
}

type cblBook struct {
	Series string `xml:"Series,attr"`
	Number string `xml:"Number,attr"`
	Volume string `xml:"Volume,attr"`
	Year   string `xml:"Year,attr"`
}

// parseCBL decodes a CBL document. A UTF-16 byte order mark is converted to
// UTF-8 first; encoding/xml itself reads only UTF-8. A declared UTF-16
// encoding without that mark is refused.
func parseCBL(raw []byte) (cblDoc, error) {
	var doc cblDoc
	data := raw
	transcoded := false
	if len(raw) >= 2 && (raw[0] == 0xFF && raw[1] == 0xFE || raw[0] == 0xFE && raw[1] == 0xFF) {
		u, err := decodeUTF16(raw)
		if err != nil {
			return doc, err
		}
		data, transcoded = u, true
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = true
	d.Entity = nil
	d.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		switch strings.ToLower(label) {
		case "utf-8", "utf8":
			return input, nil
		case "utf-16", "utf16":
			if transcoded {
				return input, nil
			}
		}
		return nil, errors.New("unsupported encoding " + label)
	}
	if err := d.Decode(&doc); err != nil {
		return doc, err
	}
	return doc, nil
}

// decodeUTF16 converts UTF-16 bytes with a byte order mark to UTF-8.
func decodeUTF16(raw []byte) ([]byte, error) {
	bigEndian := raw[0] == 0xFE
	body := raw[2:]
	if len(body)%2 != 0 {
		return nil, errors.New("odd UTF-16 length")
	}
	units := make([]uint16, len(body)/2)
	for i := range units {
		hi, lo := body[2*i], body[2*i+1]
		if bigEndian {
			units[i] = uint16(hi)<<8 | uint16(lo)
		} else {
			units[i] = uint16(lo)<<8 | uint16(hi)
		}
	}
	return []byte(string(utf16.Decode(units))), nil
}

// cblKey is the form series titles are compared in: compatibility forms
// folded (full width to ASCII, half width kana to full width), lower case,
// and all white space removed.
func cblKey(s string) string {
	var b strings.Builder
	for _, r := range norm.NFKC.String(s) {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// cblSeriesIndex maps each comparison key of a visible series (its title and
// alternate titles) to the series, in the order given (title order).
func cblSeriesIndex(series []Series) map[string][]Series {
	index := map[string][]Series{}
	add := func(title string, sr Series) {
		key := cblKey(title)
		if key == "" {
			return
		}
		for _, have := range index[key] {
			if have.Id == sr.Id {
				return
			}
		}
		index[key] = append(index[key], sr)
	}
	for _, sr := range series {
		add(sr.Title, sr)
		for _, alt := range sr.AlternateTitles {
			add(alt, sr)
		}
	}
	return index
}

// cblBook returns the visible book of a series with the given number, the
// first by number sort when there are several.
func (s *server) cblBook(ctx context.Context, seriesID string, number float64) (Book, bool, error) {
	bs, err := s.listBooks(ctx, " AND b.series_id = ? AND b.number = ?", "ORDER BY b.number_sort, b.id LIMIT 1", seriesID, number)
	if err != nil || len(bs) == 0 {
		return Book{}, false, err
	}
	return bs[0], true, nil
}

// cblMatch finds the book for one entry. Among the series whose title or
// alternate title matches and that hold the numbered book, a book whose
// release year equals the entry's Year wins; otherwise the first by title.
// Volume is not compared: a series has no volume number to compare it with.
func (s *server) cblMatch(ctx context.Context, index map[string][]Series, e CblEntry) (Book, bool, error) {
	number, err := strconv.ParseFloat(e.Number, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return Book{}, false, nil
	}
	year := e.Year
	var first, fit *Book
	for _, sr := range index[cblKey(e.Series)] {
		b, ok, err := s.cblBook(ctx, sr.Id, number)
		if err != nil {
			return Book{}, false, err
		}
		if !ok {
			continue
		}
		if first == nil {
			first = &b
		}
		if year != "" && fit == nil && b.ReleaseDate != nil && strings.HasPrefix(*b.ReleaseDate, year) {
			fit = &b
		}
	}
	switch {
	case fit != nil:
		return *fit, true, nil
	case first != nil:
		return *first, true, nil
	}
	return Book{}, false, nil
}

func (s *server) ImportReadList(ctx context.Context, req ImportReadListRequestObject) (ImportReadListResponseObject, error) {
	if req.Body == nil {
		return ImportReadList400JSONResponse{badRequest(ctx, "bad_request.body_missing")}, nil
	}
	raw, err := io.ReadAll(io.LimitReader(req.Body, maxCBL+1))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return ImportReadList400JSONResponse{badRequest(ctx, "bad_request.cbl_too_large")}, nil
		}
		return ImportReadList400JSONResponse{badRequest(ctx, "bad_request.cbl_malformed")}, nil
	}
	if len(raw) > maxCBL {
		return ImportReadList400JSONResponse{badRequest(ctx, "bad_request.cbl_too_large")}, nil
	}
	doc, err := parseCBL(raw)
	if err != nil {
		return ImportReadList400JSONResponse{badRequest(ctx, "bad_request.cbl_malformed")}, nil
	}

	name := strings.TrimSpace(doc.Name)
	if name == "" {
		name = msg(ctx, "read_list.default_name")
	}
	series, err := s.listSeries(ctx, "", "ORDER BY s.title_sort, s.id")
	if err != nil {
		return nil, err
	}
	index := cblSeriesIndex(series)

	res := CblImportResult{Name: name, Matched: []CblMatch{}, Unmatched: []CblEntry{}}
	var bookIDs []string
	for _, item := range doc.Books.Items {
		entry := CblEntry{
			Series: strings.TrimSpace(item.Series),
			Number: strings.TrimSpace(item.Number),
			Volume: strings.TrimSpace(item.Volume),
			Year:   strings.TrimSpace(item.Year),
		}
		book, ok, err := s.cblMatch(ctx, index, entry)
		if err != nil {
			return nil, err
		}
		if !ok {
			res.Unmatched = append(res.Unmatched, entry)
			continue
		}
		res.Matched = append(res.Matched, CblMatch{Entry: entry, Book: book})
		bookIDs = append(bookIDs, book.Id)
	}

	dryRun := req.Params.DryRun != nil && *req.Params.DryRun
	if dryRun {
		return ImportReadList200JSONResponse(res), nil
	}
	if len(res.Matched) == 0 {
		return ImportReadList400JSONResponse{badRequest(ctx, "bad_request.cbl_no_match")}, nil
	}
	// CreateReadList keeps the first of each book and checks the ids again,
	// so the list is built exactly as a manual create would build it.
	created, err := s.CreateReadList(ctx, CreateReadListRequestObject{Body: &CreateReadListJSONRequestBody{
		Name:    name,
		BookIds: &bookIDs,
	}})
	if err != nil {
		return nil, err
	}
	switch r := created.(type) {
	case CreateReadList201JSONResponse:
		list := ReadList(r)
		res.ReadList = &list
		return ImportReadList200JSONResponse(res), nil
	case CreateReadList400JSONResponse:
		return ImportReadList400JSONResponse{r.BadRequestJSONResponse}, nil
	}
	return nil, errors.New("unexpected create read list response")
}
