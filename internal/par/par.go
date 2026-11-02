// Package par runs independent network or subprocess jobs concurrently.
package par

import "sync"

// Map calls fn(0..n-1) with at most limit running at once and returns the results in order.
func Map[T any](n, limit int, fn func(i int) T) []T {
	out := make([]T, n)
	sem := make(chan struct{}, max(limit, 1))
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = fn(i)
		}()
	}
	wg.Wait()
	return out
}
