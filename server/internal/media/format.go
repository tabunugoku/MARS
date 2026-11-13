package media

import (
	"path/filepath"
	"strings"
)

// Format is the encoding of a thumbnail file: the server setting
// thumbnailFormat.
type Format string

const (
	FormatJPEG Format = "jpeg"
	FormatWebP Format = "webp"
	FormatAVIF Format = "avif"
)

// Ext is the file extension of the format, with its dot. An unknown format
// gives ".jpg".
func (f Format) Ext() string {
	switch f {
	case FormatWebP:
		return ".webp"
	case FormatAVIF:
		return ".avif"
	default:
		return ".jpg"
	}
}

// ParseFormat returns the format named s. An empty or unknown name gives
// FormatJPEG.
func ParseFormat(s string) Format {
	switch Format(s) {
	case FormatWebP, FormatAVIF:
		return Format(s)
	default:
		return FormatJPEG
	}
}

// ContentTypeForName is the MIME type of an image file by its extension.
// Anything but .webp and .avif (and .jpg, .jpeg) is served as image/jpeg.
func ContentTypeForName(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".webp":
		return "image/webp"
	case ".avif":
		return "image/avif"
	default:
		return "image/jpeg"
	}
}
