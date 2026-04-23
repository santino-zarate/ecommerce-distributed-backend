package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

var (
	startedAt = time.Now()
	counters  sync.Map // map[string]*atomic.Int64
)

func Inc(name string) {
	Add(name, 1)
}

func Add(name string, n int64) {
	counterAny, _ := counters.LoadOrStore(name, &atomic.Int64{})
	counter := counterAny.(*atomic.Int64)
	counter.Add(n)
}

func Snapshot() map[string]interface{} {
	out := map[string]interface{}{
		"uptime_seconds": int64(time.Since(startedAt).Seconds()),
	}

	counters.Range(func(key, value any) bool {
		name := key.(string)
		counter := value.(*atomic.Int64)
		out[name] = counter.Load()
		return true
	})

	return out
}
