package cache

import (
	"net/http"
	"testing"
)

func TestHasFileExtension(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		extensions []string
		want       bool
	}{
		{
			name:       "simple extension match - zip",
			path:       "/path/to/file.zip",
			extensions: []string{".zip", ".tar", ".gz"},
			want:       true,
		},
		{
			name:       "simple extension no match",
			path:       "/path/to/file.doc",
			extensions: []string{".zip", ".tar", ".gz"},
			want:       false,
		},
		{
			name:       "compound extension match - tar.gz",
			path:       "/path/to/file.tar.gz",
			extensions: []string{".tar.gz", ".tar", ".gz"},
			want:       true,
		},
		{
			name:       "compound extension priority - tar.gz should not match .tar",
			path:       "/path/to/file.tar.gz",
			extensions: []string{".tar"},
			want:       false,
		},
		{
			name:       "compound extension priority - .tar.gz listed first",
			path:       "/path/to/file.tar.gz",
			extensions: []string{".tar.gz", ".tar"},
			want:       true,
		},
		{
			name:       "case insensitive - ZIP",
			path:       "/path/to/file.ZIP",
			extensions: []string{".zip"},
			want:       true,
		},
		{
			name:       "case insensitive - Zip",
			path:       "/path/to/file.Zip",
			extensions: []string{".zip"},
			want:       true,
		},
		{
			name:       "case insensitive mixed case extension",
			path:       "/path/to/file.zip",
			extensions: []string{".ZIP", ".Zip"},
			want:       true,
		},
		{
			name:       "empty path",
			path:       "",
			extensions: []string{".zip"},
			want:       false,
		},
		{
			name:       "no extension",
			path:       "/path/to/file",
			extensions: []string{".zip"},
			want:       false,
		},
		{
			name:       "path with dot but not extension",
			path:       "/path/to/file/version",
			extensions: []string{".zip"},
			want:       false,
		},
		{
			name:       "empty extensions list",
			path:       "/path/to/file.zip",
			extensions: []string{},
			want:       false,
		},
		{
			name:       "url with query parameters",
			path:       "/path/to/file.tar.gz?v=1.0",
			extensions: []string{".tar.gz"},
			want:       false, // HasSuffix checks the exact end, so this won't match
		},
		{
			name:       "compound tar.xz extension",
			path:       "/path/to/file.tar.xz",
			extensions: []string{".tar.gz", ".tar.xz", ".tar"},
			want:       true,
		},
		{
			name:       "office document - docx",
			path:       "/path/to/document.docx",
			extensions: []string{".docx", ".doc"},
			want:       true,
		},
		{
			name:       "media file - jpg",
			path:       "/image.jpg",
			extensions: []string{".jpg", ".png", ".gif"},
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasFileExtension(tt.path, tt.extensions)
			if got != tt.want {
				t.Errorf("HasFileExtension() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasFileExtension_RealWorldCases(t *testing.T) {
	extensions := []string{
		".tar.gz", ".tar.xz", ".tar.bz2",
		".tar", ".zip", ".rar", ".7z", ".gz", ".bz2",
		".deb", ".rpm", ".apk", ".dmg",
		".exe", ".bin", ".msi",
		".docx", ".xlsx", ".pptx",
		".doc", ".xls", ".ppt",
		".odt", ".ods", ".odp",
		".pdf",
		".txt", ".md", ".rst", ".log", ".csv",
		".json", ".xml", ".yaml", ".yml", ".toml", ".ini",
		".mp3", ".mp4", ".avi", ".mkv", ".mov", ".flv", ".wmv",
		".jpeg", ".jpg", ".png", ".gif", ".svg", ".webp", ".bmp", ".tiff",
	}

	tests := []struct {
		path string
		want bool
	}{
		// Software release files
		{"/releases/download/v1.0/software-1.0.tar.gz", true},
		{"/releases/download/v1.0/software-1.0.zip", true},
		{"/releases/download/v1.0/software_1.0_amd64.deb", true},
		{"/releases/download/v1.0/software-1.0.exe", true},
		{"/releases/download/v1.0/software-1.0.dmg", true},
		{"/releases/download/v1.0/software-1.0.rpm", true},

		// Office documents
		{"/documents/report.docx", true},
		{"/documents/sheet.xlsx", true},
		{"/documents/slides.pptx", true},
		{"/documents/old.doc", true},
		{"/documents/odt.odt", true},
		{"/documents/manual.pdf", true},

		// Text files
		{"/README.md", true},
		{"/config.yaml", true},
		{"/data.json", true},
		{"/notes.txt", true},

		// Media files
		{"/images/photo.jpg", true},
		{"/images/photo.jpeg", true},
		{"/images/graphic.png", true},
		{"/videos/video.mp4", true},
		{"/audio/song.mp3", true},

		// Should not match
		{"/releases/download/v1.0/", false},
		{"/page.html", false},
		{"/script.js", false},
		{"/style.css", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := HasFileExtension(tt.path, extensions)
			if got != tt.want {
				t.Errorf("HasFileExtension(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single slash - no change",
			input:    "/path/to/resource",
			expected: "/path/to/resource",
		},
		{
			name:     "double slash",
			input:    "/path//to/resource",
			expected: "/path/to/resource",
		},
		{
			name:     "triple slash",
			input:    "/path///to/resource",
			expected: "/path/to/resource",
		},
		{
			name:     "multiple double slashes",
			input:    "/path//to///resource////file",
			expected: "/path/to/resource/file",
		},
		{
			name:     "leading double slashes",
			input:    "//path/to/resource",
			expected: "/path/to/resource",
		},
		{
			name:     "trailing double slashes",
			input:    "/path/to/resource//",
			expected: "/path/to/resource/index.html",
		},
		{
			name:     "trailing slash directory",
			input:    "/foo/",
			expected: "/foo/index.html",
		},
		{
			name:     "only slashes",
			input:    "////",
			expected: "/index.html",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "root path",
			input:    "/",
			expected: "/index.html",
		},
		{
			name:     "complex real-world URL",
			input:    "/repo//user///project////releases/////download/v1.0/file.tar.gz",
			expected: "/repo/user/project/releases/download/v1.0/file.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePath(tt.input)
			if got != tt.expected {
				t.Errorf("normalizePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGenerateFilePath_WithNormalization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal path",
			input:    "/path/to/file.tar.gz",
			expected: "path/to/file.tar.gz",
		},
		{
			name:     "path with double slashes",
			input:    "/path//to/file.tar.gz",
			expected: "path/to/file.tar.gz",
		},
		{
			name:     "path with multiple consecutive slashes",
			input:    "/path///to////file.tar.gz",
			expected: "path/to/file.tar.gz",
		},
		{
			name:     "root with double slashes",
			input:    "//",
			expected: "index.html",
		},
		{
			name:     "root path",
			input:    "/",
			expected: "index.html",
		},
		{
			name:     "directory path resolves to index.html",
			input:    "/foo/",
			expected: "foo/index.html",
		},
		{
			name:     "directory path and explicit index.html share storage",
			input:    "/foo/index.html",
			expected: "foo/index.html",
		},
		{
			name:     "complex GitHub release URL with extra slashes",
			input:    "/releases//download///v1.0////file.tar.gz",
			expected: "releases/download/v1.0/file.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateFilePath(tt.input)
			if got != tt.expected {
				t.Errorf("GenerateFilePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGenerateCacheKey_TrailingSlashEquivalence(t *testing.T) {
	paths := []string{
		"/",
		"//",
		"/index.html",
		"/foo/",
		"/foo//",
		"/foo/index.html",
	}

	makeKey := func(t *testing.T, path string) string {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, "http://example.com"+path, nil)
		if err != nil {
			t.Fatalf("failed to create request for %q: %v", path, err)
		}
		key, err := GenerateCacheKey(req, "example.com", false)
		if err != nil {
			t.Fatalf("GenerateCacheKey failed for %q: %v", path, err)
		}
		return key
	}

	rootKey := makeKey(t, "/index.html")
	fooKey := makeKey(t, "/foo/index.html")

	for _, path := range paths {
		key := makeKey(t, path)
		want := fooKey
		if path == "/" || path == "//" || path == "/index.html" {
			want = rootKey
		}
		if key != want {
			t.Errorf("GenerateCacheKey(%q) = %q, want %q (same as /index.html form)", path, key, want)
		}
	}
}
