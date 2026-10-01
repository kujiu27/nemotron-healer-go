package cache

import (
	"sync"
	"testing"
)

func TestConcurrentMetricAccess(t *testing.T) {
	c := NewMetricCache()
	var wg sync.WaitGroup
	workers := 10

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				c.Record("page_views", 1)
				_ = c.Get("page_views")
			}
		}()
	}

	wg.Wait()
	if total := c.Get("page_views"); total != workers*5 {
		t.Fatalf("expected total %d, got %d (race corruption)", workers*5, total)
	}
}
