package stats

import "time"

// Stats represents cache statistics for a single domain or overall.
type Stats struct {
	CacheHits      uint64    `json:"cache_hits"`
	CacheMisses    uint64    `json:"cache_misses"`
	BytesSaved     uint64    `json:"bytes_saved"`
	CacheSizeBytes uint64    `json:"cache_size_bytes"`
	ErrorResponses uint64    `json:"error_responses"`
	LastUpdated    time.Time `json:"last_updated"`
}

// AllStats represents statistics for all domains.
type AllStats struct {
	Total    Stats             `json:"total"`
	ByDomain map[string]*Stats `json:"by_domain"`
}

// NewAllStats creates a new AllStats instance.
func NewAllStats() *AllStats {
	return &AllStats{
		ByDomain: make(map[string]*Stats),
	}
}

// GetOrCreateDomainStats returns stats for a domain, creating if needed.
func (a *AllStats) GetOrCreateDomainStats(domain string) *Stats {
	if a.ByDomain[domain] == nil {
		a.ByDomain[domain] = &Stats{}
	}
	return a.ByDomain[domain]
}
