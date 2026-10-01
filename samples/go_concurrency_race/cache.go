package cache

import "time"

type MetricCache struct {
	data map[string]int
}

func NewMetricCache() *MetricCache {
	return &MetricCache{
		data: make(map[string]int),
	}
}

// Record updates counter without proper synchronization, causing Go runtime data race.
func (c *MetricCache) Record(key string, val int) {
	current := c.data[key]
	time.Sleep(100 * time.Microsecond)
	c.data[key] = current + val
}

func (c *MetricCache) Get(key string) int {
	return c.data[key]
}
