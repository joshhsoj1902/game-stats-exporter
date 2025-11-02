package api

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

// FilteredGatherer wraps a gatherer to only return metrics matching a prefix
type FilteredGatherer struct {
	gatherer prometheus.Gatherer
	prefix   string
}

func NewFilteredGatherer(gatherer prometheus.Gatherer, prefix string) *FilteredGatherer {
	return &FilteredGatherer{
		gatherer: gatherer,
		prefix:   prefix,
	}
}

func (fg *FilteredGatherer) Gather() ([]*dto.MetricFamily, error) {
	all, err := fg.gatherer.Gather()
	if err != nil {
		return nil, err
	}

	filtered := make([]*dto.MetricFamily, 0, len(all))
	for _, mf := range all {
		if mf.Name != nil && len(*mf.Name) >= len(fg.prefix) && (*mf.Name)[:len(fg.prefix)] == fg.prefix {
			filtered = append(filtered, mf)
		}
	}

	return filtered, nil
}

// ExcludedPrefixGatherer wraps a gatherer to exclude metrics matching certain prefixes
type ExcludedPrefixGatherer struct {
	gatherer prometheus.Gatherer
	excluded []string
}

func NewExcludedPrefixGatherer(gatherer prometheus.Gatherer, excluded []string) *ExcludedPrefixGatherer {
	return &ExcludedPrefixGatherer{
		gatherer: gatherer,
		excluded: excluded,
	}
}

func (eg *ExcludedPrefixGatherer) Gather() ([]*dto.MetricFamily, error) {
	all, err := eg.gatherer.Gather()
	if err != nil {
		return nil, err
	}

	filtered := make([]*dto.MetricFamily, 0, len(all))
	for _, mf := range all {
		if mf.Name == nil {
			continue
		}

		// Check if metric matches any excluded prefix
		excluded := false
		for _, prefix := range eg.excluded {
			if len(*mf.Name) >= len(prefix) && (*mf.Name)[:len(prefix)] == prefix {
				excluded = true
				break
			}
		}

		if !excluded {
			filtered = append(filtered, mf)
		}
	}

	return filtered, nil
}

// SystemMetricsHandler returns a handler that only serves system metrics (excludes application metrics)
func SystemMetricsHandler() http.Handler {
	// Exclude steam_* and osrs_* metrics, keep only system metrics (go_*, promhttp_*, process_*, etc.)
	excluded := NewExcludedPrefixGatherer(prometheus.DefaultGatherer, []string{"steam_", "osrs_"})
	return promhttp.HandlerFor(excluded, promhttp.HandlerOpts{})
}

// SteamHandler returns a handler that only serves Steam metrics
func SteamHandler() http.Handler {
	filtered := NewFilteredGatherer(prometheus.DefaultGatherer, "steam_")
	return promhttp.HandlerFor(filtered, promhttp.HandlerOpts{})
}

// OSRSHandler returns a handler that only serves OSRS metrics
func OSRSHandler() http.Handler {
	filtered := NewFilteredGatherer(prometheus.DefaultGatherer, "osrs_")
	return promhttp.HandlerFor(filtered, promhttp.HandlerOpts{})
}

