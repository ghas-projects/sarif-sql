package util

import "runtime"

// CalculateOptimalWorkers determines the optimal number of worker goroutines
// for I/O-bound tasks such as API status checks.
//
// The calculation considers:
// - CPU count (uses 5x multiplier for I/O-bound work)
// - Total number of tasks (repositories)
// - Reasonable upper and lower bounds
//
// Parameters:
//   - taskCount: the number of tasks to be processed (e.g., number of repositories)
//
// Returns the optimal number of workers to use in a worker pool.
func CalculateOptimalWorkers(taskCount int) int {
	cpuCount := runtime.NumCPU()

	// For I/O-bound work (API calls), use 5x CPU count
	// This allows multiple goroutines to wait on I/O while others work
	optimal := cpuCount * 5

	// Don't spawn more workers than tasks
	optimal = min(optimal, taskCount)

	// Cap at reasonable maximum to avoid resource exhaustion
	// and respect API rate limits
	optimal = min(optimal, 50)

	// Ensure at least 1 worker
	optimal = max(optimal, 1)

	return optimal
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the larger of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
