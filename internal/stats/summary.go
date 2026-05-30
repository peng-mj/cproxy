package stats

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// PrintSummary reads stats from file and prints ASCII table.
func PrintSummary(statsFile string) error {
	data, err := os.ReadFile(statsFile)
	if err != nil {
		return fmt.Errorf("failed to read stats file: %w", err)
	}

	var allStats AllStats
	if err := json.Unmarshal(data, &allStats); err != nil {
		return fmt.Errorf("failed to parse stats: %w", err)
	}

	printTable(&allStats)
	return nil
}

// PrintAllStats prints statistics from AllStats struct.
func PrintAllStats(allStats *AllStats) {
	printTable(allStats)
}

// printTable prints the statistics as an ASCII table.
func printTable(allStats *AllStats) {
	// Define column widths
	const (
		domainWidth = 13
		hitsWidth   = 10
		missWidth   = 10
		savedWidth  = 10
		sizeWidth   = 10
		errorsWidth = 12
	)

	// Build top border
	topBorder := strings.Repeat("─", domainWidth+hitsWidth+missWidth+savedWidth+sizeWidth+errorsWidth+15)
	fmt.Printf("┌%s┐\n", topBorder)

	// Print title row
	fmt.Printf("│%s│\n", centerText("Cache Summary", len(topBorder)))

	// Print separator
	printSeparator(domainWidth, hitsWidth, missWidth, savedWidth, sizeWidth, errorsWidth)

	// Print header row
	fmt.Printf("│ %-*s │ %-*s │ %-*s │ %-*s │ %-*s │ %-*s │\n",
		domainWidth, "Domain",
		hitsWidth, "Hits",
		missWidth, "Miss",
		savedWidth, "Saved",
		sizeWidth, "Size",
		errorsWidth, "Errors")

	// Print separator
	printSeparator(domainWidth, hitsWidth, missWidth, savedWidth, sizeWidth, errorsWidth)

	// Print data rows
	for domain, stats := range allStats.ByDomain {
		printDataRow(domain, stats, domainWidth, hitsWidth, missWidth, savedWidth, sizeWidth, errorsWidth)
	}

	// Print bottom border
	fmt.Printf("└%s┘\n", topBorder)
}

// centerText centers text within given width.
func centerText(text string, width int) string {
	if len(text) >= width {
		return text
	}
	padding := width - len(text)
	leftPadding := padding / 2
	rightPadding := padding - leftPadding
	return strings.Repeat(" ", leftPadding) + text + strings.Repeat(" ", rightPadding)
}

// printSeparator prints the table separator line.
func printSeparator(domainWidth, hitsWidth, missWidth, savedWidth, sizeWidth, errorsWidth int) {
	fmt.Printf("├%s┬%s┬%s┬%s┬%s┬%s┤\n",
		strings.Repeat("─", domainWidth+2),
		strings.Repeat("─", hitsWidth+2),
		strings.Repeat("─", missWidth+2),
		strings.Repeat("─", savedWidth+2),
		strings.Repeat("─", sizeWidth+2),
		strings.Repeat("─", errorsWidth+2))
}

// printDataRow prints a single data row.
func printDataRow(domain string, stats *Stats, domainWidth, hitsWidth, missWidth, savedWidth, sizeWidth, errorsWidth int) {
	fmt.Printf("│ %-.*s │ %*s │ %*s │ %*s │ %*s │ %*s │\n",
		domainWidth, truncate(domain, domainWidth),
		hitsWidth, FormatNumber(stats.CacheHits),
		missWidth, FormatNumber(stats.CacheMisses),
		savedWidth, FormatBytes(stats.BytesSaved),
		sizeWidth, FormatBytes(stats.CacheSizeBytes),
		errorsWidth, FormatNumber(stats.ErrorResponses))
}

// truncate truncates string to max length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
