package scproxy

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/peng-mj/scproxy/internal/cache"
)

func (ctx *RequestContext) handleCachedResponse(cached *cache.CachedResponse) {
	var responseSize uint64
	if cached.Body != nil {
		responseSize = uint64(len(cached.Body))
	} else if cached.FilePath != "" {
		if info, err := os.Stat(cached.FilePath); err == nil {
			responseSize = uint64(info.Size())
		}
	}

	if ctx.statsCollector != nil && ctx.targetHost != "" {
		ctx.statsCollector.RecordCacheHit(ctx.targetHost, responseSize)
	}

	if cached.Body != nil {
		ctx.handleMemoryResponse(cached)
		return
	}

	ctx.handleFileResponse(cached)
}

func (ctx *RequestContext) handleMemoryResponse(cached *cache.CachedResponse) {
	for k, v := range cached.Headers {
		if k == "Transfer-Encoding" || k == "Connection" {
			continue
		}
		// Rewrite Date header to current time
		if k == "Date" {
			ctx.w.Header().Set(k, time.Now().UTC().Format(http.TimeFormat))
			continue
		}
		for _, val := range v {
			ctx.w.Header().Add(k, val)
		}
	}

	rangeHeader := ctx.r.Header.Get("Range")
	if rangeHeader != "" {
		var start, end int64
		if strings.HasPrefix(rangeHeader, "bytes=") {
			rangeSpec := strings.TrimPrefix(rangeHeader, "bytes=")
			parts := strings.Split(rangeSpec, "-")
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				start, _ = strconv.ParseInt(parts[0], 10, 64)
				end, _ = strconv.ParseInt(parts[1], 10, 64)
			} else if len(parts) == 2 && parts[0] != "" && parts[1] == "" {
				start, _ = strconv.ParseInt(parts[0], 10, 64)
				end = int64(len(cached.Body)) - 1
			} else if len(parts) == 1 && parts[0] != "" {
				start, _ = strconv.ParseInt(parts[0], 10, 64)
				end = int64(len(cached.Body)) - 1
			}
		}

		fileSize := int64(len(cached.Body))
		if start < 0 || start >= fileSize || end < start || end >= fileSize {
			ctx.w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", fileSize))
			ctx.w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}

		ctx.w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
		ctx.w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
		ctx.w.WriteHeader(http.StatusPartialContent)
		ctx.w.Write(cached.Body[start : end+1])
		return
	}

	ctx.w.Header().Set("X-Cache", "HIT")
	ctx.w.WriteHeader(cached.StatusCode)
	ctx.w.Write(cached.Body)
}

func (ctx *RequestContext) handleFileResponse(cached *cache.CachedResponse) {
	file, err := os.Open(cached.FilePath)
	if err != nil {
		http.Error(ctx.w, "Failed to open cached file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		http.Error(ctx.w, "Failed to get file info", http.StatusInternalServerError)
		return
	}
	fileSize := fileInfo.Size()

	for k, v := range cached.Headers {
		if k == "Transfer-Encoding" || k == "Connection" {
			continue
		}
		// Rewrite Date header to current time
		if k == "Date" {
			ctx.w.Header().Set(k, time.Now().UTC().Format(http.TimeFormat))
			continue
		}
		for _, val := range v {
			ctx.w.Header().Add(k, val)
		}
	}

	rangeHeader := ctx.r.Header.Get("Range")
	if rangeHeader != "" {
		var start, end int64
		if strings.HasPrefix(rangeHeader, "bytes=") {
			rangeSpec := strings.TrimPrefix(rangeHeader, "bytes=")
			parts := strings.Split(rangeSpec, "-")
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				start, _ = strconv.ParseInt(parts[0], 10, 64)
				end, _ = strconv.ParseInt(parts[1], 10, 64)
			} else if len(parts) == 2 && parts[0] != "" && parts[1] == "" {
				start, _ = strconv.ParseInt(parts[0], 10, 64)
				end = fileSize - 1
			} else if len(parts) == 1 && parts[0] != "" {
				start, _ = strconv.ParseInt(parts[0], 10, 64)
				end = fileSize - 1
			}
		}

		if start < 0 || start >= fileSize || end < start || end >= fileSize {
			ctx.w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", fileSize))
			ctx.w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}

		_, err = file.Seek(start, 0)
		if err != nil {
			http.Error(ctx.w, "Failed to seek in file", http.StatusInternalServerError)
			return
		}

		ctx.w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
		ctx.w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
		ctx.w.Header().Set("X-Cache", "HIT")
		ctx.w.WriteHeader(http.StatusPartialContent)

		io.CopyN(ctx.w, file, end-start+1)
		return
	}

	ctx.w.Header().Set("X-Cache", "HIT")
	ctx.w.Header().Set("Content-Length", fmt.Sprintf("%d", fileSize))
	ctx.w.WriteHeader(cached.StatusCode)
	io.Copy(ctx.w, file)
}
