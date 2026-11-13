package httpapi

// Feed titles of the OPDS catalog (opds.go).
func init() {
	for k, v := range map[string]struct{ ja, en string }{
		"opds.catalog_title":      {"MARS", "MARS"},
		"opds.keep_reading":       {"続きを読む", "Keep reading"},
		"opds.latest_series":      {"最近追加されたシリーズ", "Recently added series"},
		"opds.latest_books":       {"最近追加された巻", "Recently added books"},
		"opds.libraries":          {"ライブラリ", "Libraries"},
		"opds.collections":        {"コレクション", "Collections"},
		"opds.readlists":          {"リーディングリスト", "Reading lists"},
		"opds.search_description": {"MARS をシリーズ名、別名、作者で検索します", "Search MARS by series title, alternate title or author"},
		"opds.search_results":     {"「%s」の検索結果", "Results for \"%s\""},
	} {
		messages[k] = v
	}
}
