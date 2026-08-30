package scproxy

import (
	"net/http"
	"net/url"
	"testing"
)

func TestRewriteDirectoryIndexPath(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		rawPath     string
		wantPath    string
		wantRawPath string
	}{
		{
			name:     "GET directory resolves to index document",
			method:   "GET",
			path:     "/foo/",
			wantPath: "/foo/index.html",
		},
		{
			name:     "HEAD directory resolves to index document",
			method:   "HEAD",
			path:     "/foo/",
			wantPath: "/foo/index.html",
		},
		{
			name:     "GET root resolves to /index.html",
			method:   "GET",
			path:     "/",
			wantPath: "/index.html",
		},
		{
			name:     "GET trailing double slash resolves to index document",
			method:   "GET",
			path:     "/foo//",
			wantPath: "/foo/index.html",
		},
		{
			name:     "GET without trailing slash untouched",
			method:   "GET",
			path:     "/foo",
			wantPath: "/foo",
		},
		{
			name:     "GET interior double slashes preserved",
			method:   "GET",
			path:     "/a//b",
			wantPath: "/a//b",
		},
		{
			name:     "POST directory untouched",
			method:   "POST",
			path:     "/dir/",
			wantPath: "/dir/",
		},
		{
			name:     "PUT directory untouched",
			method:   "PUT",
			path:     "/dir/",
			wantPath: "/dir/",
		},
		{
			name:     "DELETE directory untouched",
			method:   "DELETE",
			path:     "/dir/",
			wantPath: "/dir/",
		},
		{
			name:        "GET encoded RawPath gets matching suffix",
			method:      "GET",
			path:        "/a b/",
			rawPath:     "/a%20b/",
			wantPath:    "/a b/index.html",
			wantRawPath: "/a%20b/index.html",
		},
		{
			name:        "GET RawPath not ending with slash is cleared",
			method:      "GET",
			path:        "/foo/",
			rawPath:     "/foo%2F",
			wantPath:    "/foo/index.html",
			wantRawPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Method: tt.method,
				URL:    &url.URL{Path: tt.path, RawPath: tt.rawPath},
			}
			rewriteDirectoryIndexPath(req)
			if req.URL.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", req.URL.Path, tt.wantPath)
			}
			if req.URL.RawPath != tt.wantRawPath {
				t.Errorf("RawPath = %q, want %q", req.URL.RawPath, tt.wantRawPath)
			}
		})
	}
}
