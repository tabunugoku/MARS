package httpapi

// Catalog entries for the Komga-compatible API (komga.go).
func init() {
	for k, v := range map[string]struct{ ja, en string }{
		"unsupported.page_convert": {"このページは PNG に変換できません", "This page cannot be converted to PNG"},
	} {
		messages[k] = v
	}
}
