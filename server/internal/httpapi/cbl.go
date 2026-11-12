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

// cblCatalog is what one import reads of its candidate series, once: the
// visible books of each series by number (the first in number sort order
// wins, as a single lookup would pick it), and each series' start year.
type cblCatalog struct {
	byNumber  map[string]map[float64]Book
	startYear map[string]string
}

// loadCBLCatalog reads the visible books of every series any entry can match
// in one query. The start year of a series is the release year of its earliest
// book that has one, since a series carries no year of its own.
func (s *server) loadCBLCatalog(ctx context.Context, index map[string][]Series, items []cblBook) (*cblCatalog, error) {
	cat := &cblCatalog{byNumber: map[string]map[float64]Book{}, startYear: map[string]string{}}
	var ids []string
	seen := map[string]bool{}
	for _, it := range items {
		for _, sr := range index[cblKey(strings.TrimSpace(it.Series))] {
			if !seen[sr.Id] {
				seen[sr.Id] = true
				ids = append(ids, sr.Id)
			}
		}
	}
	if len(ids) == 0 {
		return cat, nil
	}
	books, err := s.listBooks(ctx, " AND b.series_id IN ("+placeholders(len(ids))+")",
		"ORDER BY b.number_sort, b.id", anyStrings(ids)...)
	if err != nil {
		return nil, err
	}
	for _, b := range books {
		nums := cat.byNumber[b.SeriesId]
		if nums == nil {
			nums = map[float64]Book{}
			cat.byNumber[b.SeriesId] = nums
		}
		if _, ok := nums[b.Number]; !ok {
			nums[b.Number] = b
		}
		if _, ok := cat.startYear[b.SeriesId]; !ok && b.ReleaseDate != nil && len(*b.ReleaseDate) >= 4 {
			cat.startYear[b.SeriesId] = (*b.ReleaseDate)[:4]
		}
	}
	return cat, nil
}

// match finds the book for one entry. Among the series whose title or
// alternate title matches and that hold the numbered book, a series that
// started in the entry's Volume wins when there is one; then a book whose
// release year equals the entry's Year; otherwise the first by title.
func (cat *cblCatalog) match(index map[string][]Series, e CblEntry) (Book, bool) {
	number, err := strconv.ParseFloat(e.Number, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return Book{}, false
	}
	type candidate struct {
		seriesID string
		book     Book
	}
	var cands []candidate
	for _, sr := range index[cblKey(e.Series)] {
		if b, ok := cat.byNumber[sr.Id][number]; ok {
			cands = append(cands, candidate{sr.Id, b})
		}
	}
	if len(cands) == 0 {
		return Book{}, false
	}
	if e.Volume != "" {
		var same []candidate
		for _, c := range cands {
			if cat.startYear[c.seriesID] == e.Volume {
				same = append(same, c)
			}
		}
		if len(same) > 0 {
			cands = same
		}
	}
	for _, c := range cands {
		if e.Year != "" && c.book.ReleaseDate != nil && strings.HasPrefix(*c.book.ReleaseDate, e.Year) {
			return c.book, true
		}
	}
	return cands[0].book, true
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
	cat, err := s.loadCBLCatalog(ctx, index, doc.Books.Items)
	if err != nil {
		return nil, err
	}

	res := CblImportResult{Name: name, Matched: []CblMatch{}, Unmatched: []CblEntry{}}
	var bookIDs []string
	for _, item := range doc.Books.Items {
		entry := CblEntry{
			Series: strings.TrimSpace(item.Series),
			Number: strings.TrimSpace(item.Number),
			Volume: strings.TrimSpace(item.Volume),
			Year:   strings.TrimSpace(item.Year),
		}
		book, ok := cat.match(index, entry)
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
