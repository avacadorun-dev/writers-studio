package httpapi

import "testing"

func TestSafeUploadedFileName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value string
		want  string
	}{
		"unicode":   {value: "%D0%BA%D0%B0%D1%80%D1%82%D0%B0.png", want: "карта.png"},
		"path":      {value: "..%2F..%2Fsecret.png", want: "secret.png"},
		"windows":   {value: "folder%5Cmap.png", want: "map.png"},
		"malformed": {value: "%zz", want: "image.png"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := safeUploadedFileName(test.value, ".png"); got != test.want {
				t.Fatalf("safeUploadedFileName() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestValidPDFChecksHeaderAndTrailer(t *testing.T) {
	t.Parallel()
	if !validPDF([]byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n")) {
		t.Fatal("minimal PDF was rejected")
	}
	for _, invalid := range [][]byte{
		[]byte("not a pdf %%EOF"),
		[]byte("%PDF-1.7 without trailer"),
		[]byte("%PDF-"),
	} {
		if validPDF(invalid) {
			t.Fatalf("invalid PDF accepted: %q", invalid)
		}
	}
	large := append([]byte("%PDF-1.7\n"), make([]byte, 5_000)...)
	large = append(large, []byte("%%EOF")...)
	if !validPDF(large) {
		t.Fatal("PDF trailer outside the first block was rejected")
	}
}

func TestNormalizedUploadedMediaType(t *testing.T) {
	t.Parallel()
	mp4 := append([]byte{0, 0, 0, 20}, []byte("ftypisom")...)
	if got := normalizedUploadedMediaType("application/octet-stream", "audio/mp4", mp4); got != "audio/mp4" {
		t.Fatalf("M4A type = %q", got)
	}
	if got := normalizedUploadedMediaType("application/octet-stream", "video/mp4", mp4); got != "video/mp4" {
		t.Fatalf("MP4 type = %q", got)
	}
	if got := normalizedUploadedMediaType("application/ogg", "audio/ogg; codecs=opus", []byte("OggS")); got != "audio/ogg" {
		t.Fatalf("OGG type = %q", got)
	}
	if got := normalizedUploadedMediaType("audio/x-wav", "", []byte("RIFF")); got != "audio/wav" {
		t.Fatalf("WAV type = %q", got)
	}
	if got := normalizedUploadedMediaType("audio/mpeg", "audio/mpeg", []byte("ID3")); got != "audio/mpeg" {
		t.Fatalf("MP3 type = %q", got)
	}
}
