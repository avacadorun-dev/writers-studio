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
