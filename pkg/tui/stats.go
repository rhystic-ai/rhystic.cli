package tui

import (
	"fmt"
	"time"
)

// DashboardStats holds aggregate statistics for the dashboard
type DashboardStats struct {
	TotalRuns    int
	SuccessCount int
	FailCount    int
	PartialCount int
	PendingCount int
	TotalNodes   int
	SuccessRate  float64
	FailRate     float64
	LastRunTime  time.Time
}

// CalculateStats computes statistics from a slice of runs
func CalculateStats(runs []RunSummary) DashboardStats {
	stats := DashboardStats{
		TotalRuns: len(runs),
	}

	if len(runs) == 0 {
		return stats
	}

	for _, run := range runs {
		switch run.Status {
		case "success":
			stats.SuccessCount++
		case "fail":
			stats.FailCount++
		case "partial_success":
			stats.PartialCount++
		default:
			stats.PendingCount++
		}

		stats.TotalNodes += run.NodeCount

		// Track most recent run
		if run.CompletedAt.After(stats.LastRunTime) {
			stats.LastRunTime = run.CompletedAt
		}
	}

	// Calculate percentages
	if stats.TotalRuns > 0 {
		stats.SuccessRate = float64(stats.SuccessCount) / float64(stats.TotalRuns) * 100
		stats.FailRate = float64(stats.FailCount) / float64(stats.TotalRuns) * 100
	}

	return stats
}

// FormatRate formats a percentage for display
func FormatRate(rate float64) string {
	return fmt.Sprintf("%.0f%%", rate)
}

// FormatNumber formats a number with commas
func FormatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%dK", n/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000)
}

// FormatDurationHuman formats duration in human readable form
func FormatDurationHuman(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// truncateString truncates a string to maxLen
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// GetRecentRuns filters runs from the last N duration
func GetRecentRuns(runs []RunSummary, duration time.Duration) []RunSummary {
	cutoff := time.Now().Add(-duration)
	var recent []RunSummary
	for _, run := range runs {
		if run.CompletedAt.After(cutoff) {
			recent = append(recent, run)
		}
	}
	return recent
}
