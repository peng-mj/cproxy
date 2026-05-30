package stats

import (
	"fmt"
)

const (
	KB = 1024
	MB = 1024 * KB
	GB = 1024 * MB
)

// FormatBytes converts bytes to human-readable format with 1 decimal place.
// - < 1 MB: "X.X KB"
// - >= 1 MB and < 1 GB: "X.X MB"
// - >= 1 GB: "X.X GB"
func FormatBytes(bytes uint64) string {
	if bytes < MB {
		return fmt.Sprintf("%.1f KB", float64(bytes)/KB)
	} else if bytes < GB {
		return fmt.Sprintf("%.1f MB", float64(bytes)/MB)
	} else {
		return fmt.Sprintf("%.1f GB", float64(bytes)/GB)
	}
}

// FormatNumber formats a number with thousand separators.
func FormatNumber(n uint64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d", n)
}
