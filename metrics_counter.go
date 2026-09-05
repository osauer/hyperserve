package hyperserve

import (
	"math/rand/v2"
	"sync/atomic"
)

// A separate cache line per stripe keeps concurrent requests from repeatedly
// transferring ownership of the same two counters between CPUs. Reads aggregate
// exact totals; there is no sampling. Replacing the stripe set makes Store a
// reset boundary without racing a sequence of individual stripe resets.
type metricCounter[T ~int64 | ~uint64] struct {
	stripes atomic.Pointer[metricStripes]
}

type metricStripe struct {
	value atomic.Uint64
	_     [120]byte
}
type metricStripes [32]metricStripe

func (c *metricCounter[T]) init() *metricStripes {
	if stripes := c.stripes.Load(); stripes != nil {
		return stripes
	}
	stripes := new(metricStripes)
	if c.stripes.CompareAndSwap(nil, stripes) {
		return stripes
	}
	return c.stripes.Load()
}
func (c *metricCounter[T]) Add(delta T) {
	if delta == 0 {
		return
	}
	stripes := c.init()
	stripes[rand.Uint32()&31].value.Add(uint64(delta))
}
func (c *metricCounter[T]) Load() T {
	stripes := c.stripes.Load()
	if stripes == nil {
		return 0
	}
	var sum uint64
	for i := range stripes {
		sum += stripes[i].value.Load()
	}
	return T(sum)
}
func (c *metricCounter[T]) Store(value T) {
	stripes := new(metricStripes)
	stripes[0].value.Store(uint64(value))
	c.stripes.Store(stripes)
}
