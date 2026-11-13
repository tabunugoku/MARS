package media

import "testing"

func TestFormatExtAndParse(t *testing.T) {
	cases := map[Format]string{FormatJPEG: ".jpg", FormatWebP: ".webp", FormatAVIF: ".avif", Format("x"): ".jpg"}
	for f, want := range cases {
		if got := f.Ext(); got != want {
			t.Errorf("Format(%q).Ext() = %q, want %q", f, got, want)
		}
	}
	for in, want := range map[string]Format{"webp": FormatWebP, "avif": FormatAVIF, "jpeg": FormatJPEG, "": FormatJPEG, "png": FormatJPEG} {
		if got := ParseFormat(in); got != want {
			t.Errorf("ParseFormat(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestContentTypeForName(t *testing.T) {
	for name, want := range map[string]string{
		"a-1.jpg": "image/jpeg", "a.JPEG": "image/jpeg", "a.webp": "image/webp",
		"a.avif": "image/avif", "a.bin": "image/jpeg", "a": "image/jpeg",
	} {
		if got := ContentTypeForName(name); got != want {
			t.Errorf("ContentTypeForName(%q) = %q, want %q", name, got, want)
		}
	}
}
