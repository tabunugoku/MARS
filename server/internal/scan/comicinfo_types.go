package scan

// Author matches the web contract's Author (role is one of writer,
// penciller, colorist, translator, editor, cover).
type Author struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

// ComicInfo holds the ComicInfo.xml fields MARS uses (Anansi schema
// v2.0; unknown elements are ignored). Empty strings and zero numbers mean
// "not given".
type ComicInfo struct {
	Title       string
	Series      string
	Number      string
	Count       int
	Summary     string
	Year        int
	Month       int
	Day         int
	Writer      string // comma-separated names, as in the schema
	Penciller   string
	Inker       string
	Colorist    string
	Letterer    string
	CoverArtist string
	Editor      string
	Translator  string
	Publisher   string
	Genre       string // comma-separated
	Tags        string // comma-separated
	LanguageISO string
	Manga       string // Unknown | No | Yes | YesAndRightToLeft
	AgeRating   string
	GTIN        string
}
