package stats

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/brettski/go-termtables"
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

// printTable prints the statistics as an ASCII table using go-termtables.
func printTable(allStats *AllStats) {
	table := termtables.CreateTable()
	table.AddHeaders("Domain", "Hits", "Miss", "Saved", "Size", "Errors")

	for domain, stats := range allStats.ByDomain {
		table.AddRow(
			domain,
			FormatNumber(stats.CacheHits),
			FormatNumber(stats.CacheMisses),
			FormatBytes(stats.BytesSaved),
			FormatBytes(stats.CacheSizeBytes),
			FormatNumber(stats.ErrorResponses),
		)
	}

	fmt.Println(table.Render())
}