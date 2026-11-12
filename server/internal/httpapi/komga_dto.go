package httpapi

import (
	"fmt"
	"path"
	"strconv"
)

// The Komga-shaped JSON types and their converters from our own DTOs. Field
// names and types follow the Komga API as the Mihon/Tachiyomi extension and
// the Mihon tracker parse it (docs/opds-komga.md, "Komga-shaped API"). Every
// field those clients read, or declare without a default, is present. Values
// we do not keep (file dates, read-list and collection dates, lock flags) are
// fixed placeholders: komgaEpoch for a date, false for a flag, "" or [] for
// text and lists.

// komgaEpoch stands in for a date we do not keep. It is an ISO-8601 instant
// with Z, which the extension's date parser accepts.
const komgaEpoch = "1970-01-01T00:00:00Z"

// KomgaPage is Komga's page wrapper (Spring's Page). Paged lists and the
// unpaged=true answers share it.
type KomgaPage[T any] struct {
	Content          []T           `json:"content"`
	Pageable         KomgaPageable `json:"pageable"`
	TotalElements    int           `json:"totalElements"`
	TotalPages       int           `json:"totalPages"`
	Last             bool          `json:"last"`
	First            bool          `json:"first"`
	Number           int           `json:"number"`
	Size             int           `json:"size"`
	NumberOfElements int           `json:"numberOfElements"`
	Empty            bool          `json:"empty"`
	Sort             KomgaSort     `json:"sort"`
}

type KomgaPageable struct {
	Sort       KomgaSort `json:"sort"`
	Offset     int       `json:"offset"`
	PageNumber int       `json:"pageNumber"`
	PageSize   int       `json:"pageSize"`
	Paged      bool      `json:"paged"`
	Unpaged    bool      `json:"unpaged"`
}

type KomgaSort struct {
	Empty    bool `json:"empty"`
	Sorted   bool `json:"sorted"`
	Unsorted bool `json:"unsorted"`
}

// komgaSorted is the sort every list reports: we always order a list.
var komgaSorted = KomgaSort{Empty: false, Sorted: true, Unsorted: false}

// komgaWrap builds the page wrapper for content, the items of the whole list
// (total) cut to page pg. Content is never null.
func komgaWrap[T any](content []T, total int, pg komgaPaging) KomgaPage[T] {
	if content == nil {
		content = []T{}
	}
	out := KomgaPage[T]{
		Content:          content,
		TotalElements:    total,
		NumberOfElements: len(content),
		Empty:            len(content) == 0,
		Sort:             komgaSorted,
	}
	if pg.unpaged {
		out.Size = total
		out.TotalPages = 1
		out.First, out.Last = true, true
		out.Pageable = KomgaPageable{Sort: komgaSorted, PageSize: total, Unpaged: true}
		return out
	}
	out.Size = pg.size
	out.Number = pg.page
	out.TotalPages = (total + pg.size - 1) / pg.size
	out.First = pg.page == 0
	out.Last = pg.page >= out.TotalPages-1
	out.Pageable = KomgaPageable{Sort: komgaSorted, Offset: pg.page * pg.size, PageNumber: pg.page, PageSize: pg.size, Paged: true}
	return out
}

// komgaMap converts every item; the result is never null.
func komgaMap[T, U any](in []T, f func(T) U) []U {
	out := make([]U, 0, len(in))
	for _, x := range in {
		out = append(out, f(x))
	}
	return out
}

// ─── Series ──────────────────────────────────────────────────────────────────

type KomgaSeriesDto struct {
	Id                   string                          `json:"id"`
	LibraryId            string                          `json:"libraryId"`
	Name                 string                          `json:"name"`
	Created              *string                         `json:"created"`
	LastModified         *string                         `json:"lastModified"`
	FileLastModified     string                          `json:"fileLastModified"`
	BooksCount           int                             `json:"booksCount"`
	BooksReadCount       int                             `json:"booksReadCount"`
	BooksUnreadCount     int                             `json:"booksUnreadCount"`
	BooksInProgressCount int                             `json:"booksInProgressCount"`
	Metadata             KomgaSeriesMetadataDto          `json:"metadata"`
	BooksMetadata        KomgaBookMetadataAggregationDto `json:"booksMetadata"`
}

type KomgaSeriesMetadataDto struct {
	Status               string   `json:"status"`
	Created              *string  `json:"created"`
	LastModified         *string  `json:"lastModified"`
	Title                string   `json:"title"`
	TitleSort            string   `json:"titleSort"`
	Summary              string   `json:"summary"`
	SummaryLock          bool     `json:"summaryLock"`
	ReadingDirection     string   `json:"readingDirection"`
	ReadingDirectionLock bool     `json:"readingDirectionLock"`
	Publisher            string   `json:"publisher"`
	PublisherLock        bool     `json:"publisherLock"`
	AgeRating            *int     `json:"ageRating"`
	AgeRatingLock        bool     `json:"ageRatingLock"`
	Language             string   `json:"language"`
	LanguageLock         bool     `json:"languageLock"`
	Genres               []string `json:"genres"`
	GenresLock           bool     `json:"genresLock"`
	Tags                 []string `json:"tags"`
	TagsLock             bool     `json:"tagsLock"`
	TotalBookCount       *int     `json:"totalBookCount"`
}

type KomgaBookMetadataAggregationDto struct {
	Authors       []KomgaAuthorDto `json:"authors"`
	Tags          []string         `json:"tags"`
	ReleaseDate   *string          `json:"releaseDate"`
	Summary       string           `json:"summary"`
	SummaryNumber string           `json:"summaryNumber"`
	Created       string           `json:"created"`
	LastModified  string           `json:"lastModified"`
}

type KomgaAuthorDto struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

// komgaSeries converts a series. Komga's "name" is the folder name; we keep
// only the title, so both carry it.
func komgaSeries(x Series) KomgaSeriesDto {
	unread := max(x.BookCount-x.BooksRead-x.BooksInProgress, 0)
	return KomgaSeriesDto{
		Id:                   x.Id,
		LibraryId:            x.LibraryId,
		Name:                 x.Title,
		Created:              &x.CreatedAt,
		LastModified:         &x.UpdatedAt,
		FileLastModified:     komgaEpoch,
		BooksCount:           x.BookCount,
		BooksReadCount:       x.BooksRead,
		BooksUnreadCount:     unread,
		BooksInProgressCount: x.BooksInProgress,
		Metadata: KomgaSeriesMetadataDto{
			Status:           komgaSeriesStatus(x.Status),
			Created:          &x.CreatedAt,
			LastModified:     &x.UpdatedAt,
			Title:            x.Title,
			TitleSort:        x.TitleSort,
			Summary:          x.Summary,
			ReadingDirection: komgaReadingDirection(x.ReadingDirection),
			Publisher:        x.Publisher,
			AgeRating:        x.AgeRating,
			Language:         x.Language,
			Genres:           orEmpty(x.Genres),
			Tags:             orEmpty(x.Tags),
			TotalBookCount:   x.TotalBookCount,
		},
		BooksMetadata: KomgaBookMetadataAggregationDto{
			Authors:      komgaAuthors(x.Authors),
			Tags:         orEmpty(x.Tags),
			Summary:      x.Summary,
			Created:      x.CreatedAt,
			LastModified: x.UpdatedAt,
		},
	}
}

// komgaSeriesStatus maps our status to Komga's. The enum names are upper case.
func komgaSeriesStatus(s SeriesStatus) string {
	switch s {
	case Ongoing:
		return "ONGOING"
	case Ended:
		return "ENDED"
	case Hiatus:
		return "HIATUS"
	case Abandoned:
		return "ABANDONED"
	}
	return "UNKNOWN"
}

func komgaReadingDirection(d ReadingDirection) string {
	switch d {
	case ReadingDirectionLtr:
		return "LEFT_TO_RIGHT"
	case ReadingDirectionRtl:
		return "RIGHT_TO_LEFT"
	case ReadingDirectionVertical:
		return "VERTICAL"
	case ReadingDirectionWebtoon:
		return "WEBTOON"
	}
	return "LEFT_TO_RIGHT"
}

func komgaAuthors(in []Author) []KomgaAuthorDto {
	return komgaMap(in, func(a Author) KomgaAuthorDto {
		return KomgaAuthorDto{Name: a.Name, Role: string(a.Role)}
	})
}

// ─── Books ───────────────────────────────────────────────────────────────────

type KomgaBookDto struct {
	Id               string                `json:"id"`
	SeriesId         string                `json:"seriesId"`
	SeriesTitle      string                `json:"seriesTitle"`
	Name             string                `json:"name"`
	Number           float64               `json:"number"`
	Created          *string               `json:"created"`
	LastModified     *string               `json:"lastModified"`
	FileLastModified string                `json:"fileLastModified"`
	SizeBytes        int64                 `json:"sizeBytes"`
	Size             string                `json:"size"`
	Media            KomgaMediaDto         `json:"media"`
	Metadata         KomgaBookMetadataDto  `json:"metadata"`
	ReadProgress     *KomgaReadProgressDto `json:"readProgress"`
}

type KomgaMediaDto struct {
	Status               string `json:"status"`
	MediaType            string `json:"mediaType"`
	PagesCount           int    `json:"pagesCount"`
	MediaProfile         string `json:"mediaProfile"`
	EpubDivinaCompatible bool   `json:"epubDivinaCompatible"`
}

type KomgaBookMetadataDto struct {
	Title           string           `json:"title"`
	TitleLock       bool             `json:"titleLock"`
	Summary         string           `json:"summary"`
	SummaryLock     bool             `json:"summaryLock"`
	Number          string           `json:"number"`
	NumberLock      bool             `json:"numberLock"`
	NumberSort      float64          `json:"numberSort"`
	NumberSortLock  bool             `json:"numberSortLock"`
	ReleaseDate     *string          `json:"releaseDate"`
	ReleaseDateLock bool             `json:"releaseDateLock"`
	Authors         []KomgaAuthorDto `json:"authors"`
	AuthorsLock     bool             `json:"authorsLock"`
	Tags            []string         `json:"tags"`
	TagsLock        bool             `json:"tagsLock"`
}

type KomgaReadProgressDto struct {
	Page         int    `json:"page"`
	Completed    bool   `json:"completed"`
	ReadDate     string `json:"readDate"`
	Created      string `json:"created"`
	LastModified string `json:"lastModified"`
}

// komgaBook converts a book. The file name is the last part of its path.
func komgaBook(x Book) KomgaBookDto {
	out := KomgaBookDto{
		Id:               x.Id,
		SeriesId:         x.SeriesId,
		SeriesTitle:      x.SeriesTitle,
		Name:             path.Base(x.Path),
		Number:           x.Number,
		Created:          &x.CreatedAt,
		FileLastModified: komgaEpoch,
		SizeBytes:        x.SizeBytes,
		Size:             komgaSize(x.SizeBytes),
		Media: KomgaMediaDto{
			Status:               komgaMediaStatus(x.MediaStatus),
			MediaType:            komgaMediaType(x.Format),
			PagesCount:           x.PageCount,
			MediaProfile:         komgaMediaProfile(x.Format),
			EpubDivinaCompatible: false,
		},
		Metadata: KomgaBookMetadataDto{
			Title:       x.Title,
			Summary:     x.Summary,
			Number:      strconv.FormatFloat(x.Number, 'f', -1, 64),
			NumberSort:  x.NumberSort,
			ReleaseDate: x.ReleaseDate,
			Authors:     komgaAuthors(x.Authors),
			Tags:        orEmpty(x.Tags),
		},
	}
	if x.Progress != nil {
		out.ReadProgress = &KomgaReadProgressDto{
			Page:         x.Progress.Page,
			Completed:    x.Progress.Completed,
			ReadDate:     x.Progress.ReadAt,
			Created:      x.Progress.ReadAt,
			LastModified: x.Progress.ReadAt,
		}
	}
	return out
}

func komgaMediaStatus(s MediaStatus) string {
	switch s {
	case MediaStatusReady:
		return "READY"
	case MediaStatusError:
		return "ERROR"
	case MediaStatusUnsupported:
		return "UNSUPPORTED"
	case MediaStatusOutdated:
		return "OUTDATED"
	}
	return "UNKNOWN"
}

func komgaMediaType(f BookFormat) string {
	switch f {
	case Cbz:
		return "application/zip"
	case Cbr:
		return "application/x-rar-compressed"
	case Cb7:
		return "application/x-7z-compressed"
	case Pdf:
		return "application/pdf"
	case Epub:
		return "application/epub+zip"
	}
	return ""
}

// komgaMediaProfile is Komga's profile of the file: DIVINA for pages, PDF or
// EPUB for those formats (the extension drops EPUB books it cannot read as
// pages).
func komgaMediaProfile(f BookFormat) string {
	switch f {
	case Pdf:
		return "PDF"
	case Epub:
		return "EPUB"
	}
	return "DIVINA"
}

// komgaSize formats a byte count the way Komga does ("5.7 MiB").
func komgaSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ─── Pages ───────────────────────────────────────────────────────────────────

type KomgaPageDto struct {
	Number    int    `json:"number"`
	FileName  string `json:"fileName"`
	MediaType string `json:"mediaType"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

// ─── Libraries, collections, read lists ──────────────────────────────────────

type KomgaLibraryDto struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

func komgaLibrary(l Library) KomgaLibraryDto {
	return KomgaLibraryDto{Id: l.Id, Name: l.Name}
}

type KomgaCollectionDto struct {
	Id               string   `json:"id"`
	Name             string   `json:"name"`
	Ordered          bool     `json:"ordered"`
	SeriesIds        []string `json:"seriesIds"`
	CreatedDate      string   `json:"createdDate"`
	LastModifiedDate string   `json:"lastModifiedDate"`
	Filtered         bool     `json:"filtered"`
}

func komgaCollection(c Collection) KomgaCollectionDto {
	return KomgaCollectionDto{
		Id:               c.Id,
		Name:             c.Name,
		Ordered:          c.Ordered,
		SeriesIds:        orEmpty(c.SeriesIds),
		CreatedDate:      komgaEpoch,
		LastModifiedDate: komgaEpoch,
	}
}

type KomgaReadListDto struct {
	Id               string   `json:"id"`
	Name             string   `json:"name"`
	Summary          string   `json:"summary"`
	BookIds          []string `json:"bookIds"`
	CreatedDate      string   `json:"createdDate"`
	LastModifiedDate string   `json:"lastModifiedDate"`
	Filtered         bool     `json:"filtered"`
}

func komgaReadList(l ReadList) KomgaReadListDto {
	return KomgaReadListDto{
		Id:               l.Id,
		Name:             l.Name,
		Summary:          l.Summary,
		BookIds:          orEmpty(l.BookIds),
		CreatedDate:      komgaEpoch,
		LastModifiedDate: komgaEpoch,
	}
}
