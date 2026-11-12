package httpapi

// Catalog entries for the bulk operations (bulk.go).
func init() {
	for k, v := range map[string]struct{ ja, en string }{
		"bad_request.ids_required":      {"対象を1つ以上選んでください", "Choose at least one item"},
		"bad_request.ids_too_many":      {"一度に変更できるのは1000件までです", "You can change up to 1000 items at once"},
		"bad_request.nothing_to_change": {"変更する内容を指定してください", "Choose at least one change"},
		"bad_request.read_required":     {"read は true か false で指定してください", "Set read to true or false"},
	} {
		messages[k] = v
	}
}
