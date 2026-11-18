package main

import (
	"strconv"
	"sync"
	"time"
)

// opQueueCap bounds one page's pending operations; overflow is dropped on the
// capture path (and retried on the lossless replay path).
const opQueueCap = 256

// defaultSyncDelay paces continuous operations (input/scroll) so a typing or
// scrolling burst replays as a smooth stream instead of one eval per event.
const defaultSyncDelay = 100 * time.Millisecond

// syncDelayOptions are the setup panel's pace choices in ms ("0" = instant).
var syncDelayOptions = []string{"0", "100", "250", "500"}

// syncDelayFromMs clamps a caller-supplied pace into 0–2s; nil = default.
func syncDelayFromMs(ms *float64) time.Duration {
	if ms == nil {
		return defaultSyncDelay
	}
	return time.Duration(min(max(*ms, 0), 2000)) * time.Millisecond
}

// formSyncDelay parses the setup form's pace selection; invalid = default.
func formSyncDelay(v string) time.Duration {
	if ms, err := strconv.ParseFloat(v, 64); err == nil {
		return syncDelayFromMs(&ms)
	}
	return defaultSyncDelay
}

// opQueue is one page's ordered replay queue (P0-3 delayed sync). Consecutive
// input/scroll operations on the same anchor coalesce into the latest one, and
// continuous operations pop no faster than delay apart, so a burst replays as
// a throttled stream ending at the final value/position. Discrete operations
// (click/change/select/submit) and navigation barriers always pop immediately.
// A closed queue drains its remaining operations, then reports exhaustion.
type opQueue struct {
	mu     sync.Mutex
	ops    []operation
	wake   chan struct{} // buffered ping on push/close; never lost while waiting
	closed bool
	delay  time.Duration

	// lastStream is when the last input/scroll popped, the pace clock. Discrete
	// operations do not touch it, so a click never delays the next keystroke.
	lastStream time.Time
}

func newOpQueue(delay time.Duration) *opQueue {
	return &opQueue{wake: make(chan struct{}, 1), delay: delay}
}

// push appends op, or replaces the pending tail when both are continuous
// operations of the same type on the same anchor. It never blocks: a full
// queue drops the op (returns false), except that a coalescing replace still
// succeeds because it does not grow the queue.
func (q *opQueue) push(op operation) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	if op.Type == "input" || op.Type == "scroll" {
		if n := len(q.ops); n > 0 {
			if tail := &q.ops[n-1]; tail.Type == op.Type && tail.Anchor == op.Anchor {
				*tail = op
				return true
			}
		}
	}
	if len(q.ops) >= opQueueCap {
		return false
	}
	q.ops = append(q.ops, op)
	q.ping()
	return true
}

// pop removes and returns the head operation, blocking while the queue is
// empty. A continuous head waits out the remaining pace interval first — still
// queued, so pushes keep coalescing into it — then returns the latest state.
// It reports false once the queue is closed and drained.
func (q *opQueue) pop() (operation, bool) {
	for {
		q.mu.Lock()
		if len(q.ops) == 0 {
			closed := q.closed
			q.mu.Unlock()
			if closed {
				return operation{}, false
			}
			<-q.wake
			continue
		}
		op := q.ops[0]
		var wait time.Duration
		if q.delay > 0 && (op.Type == "input" || op.Type == "scroll") {
			wait = q.delay - time.Since(q.lastStream)
		}
		if wait > 0 {
			q.mu.Unlock()
			select {
			case <-q.wake:
			case <-time.After(wait):
			}
			continue
		}
		q.ops[0] = operation{}
		q.ops = q.ops[1:]
		if op.Type == "input" || op.Type == "scroll" {
			q.lastStream = time.Now()
		}
		q.mu.Unlock()
		return op, true
	}
}

// len reports the pending operation count (barriers included).
func (q *opQueue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.ops)
}

// close marks the queue closed; pop drains what remains, then reports false.
func (q *opQueue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.closed = true
		q.ping()
	}
}

// ping wakes a waiting popper. The caller holds q.mu; the buffered channel
// keeps the signal even when nobody is waiting yet.
func (q *opQueue) ping() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}
