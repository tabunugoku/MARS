package httpapi

// Messages for covers: the cover book choice and uploaded images.
func init() {
	for k, v := range map[string]struct{ ja, en string }{
		"bad_request.cover_book":      {"表紙にする本は、このシリーズの本から選んでください", "Choose the cover book from this series"},
		"bad_request.image_format":    {"JPEG、PNG、WebP の画像を選んでください", "Choose a JPEG, PNG or WebP image"},
		"bad_request.image_size":      {"画像は縦横とも 20000 ピクセル以下にしてください", "The image must be 20000 pixels or less on each side"},
		"bad_request.image_too_large": {"画像は 20 MB 以下にしてください", "The image must be 20 MB or less"},
	} {
		messages[k] = v
	}
}
