package micro

import (
	"container/heap"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const infinite = time.Duration(math.MaxInt64)

type rpccontext struct {
	state  int32 // 0: undone, 1: done
	expire int64
	hidx   int
	code   int
	fn     Callback
	method string
	id     uint64
	result []byte
}

func (c *rpccontext) done() bool {
	return atomic.CompareAndSwapInt32(&c.state, 0, 1)
}

type contextManager struct {
	c      *Client
	mu     sync.Mutex
	ctxs   map[uint64]*rpccontext // reqid => request
	wait   []*rpccontext
	wakeAt int64
	timer  *time.Timer
}

func (m *contextManager) Len() int {
	return len(m.wait)
}

func (m *contextManager) Less(i, j int) bool {
	return m.wait[i].expire < m.wait[j].expire
}

func (m *contextManager) Swap(i, j int) {
	q := m.wait
	t := q[i]
	q[i] = q[j]
	q[i].hidx = i
	q[j] = t
	t.hidx = j
}

func (m *contextManager) Push(x interface{}) {
	c := x.(*rpccontext)
	c.hidx = len(m.wait)
	m.wait = append(m.wait, c)
}

func (m *contextManager) Pop() interface{} {
	q := m.wait
	x := q[len(q)-1]
	m.wait = q[:len(q)-1]
	return x
}

func (m *contextManager) resetTimer() {
	if len(m.wait) > 0 {
		if expire := m.wait[0].expire; expire != m.wakeAt {
			m.wakeAt = expire
			m.timer.Reset(time.Duration(expire - time.Now().UnixNano()))
		}
	} else {
		m.wakeAt = 0
		m.timer.Reset(infinite)
	}
}

func (m *contextManager) add(reqid uint64, method string, fn Callback, timeout time.Duration) *rpccontext {
	ctx := &rpccontext{
		method: method,
		id:     reqid,
		fn:     fn,
	}
	m.mu.Lock()
	ctx.expire = time.Now().Add(timeout).UnixNano()
	m.ctxs[reqid] = ctx
	heap.Push(m, ctx)
	m.resetTimer()
	m.mu.Unlock()
	return ctx
}

func (m *contextManager) remove(reqid uint64) *rpccontext {
	m.mu.Lock()
	ctx := m.ctxs[reqid]
	if ctx != nil {
		heap.Remove(m, ctx.hidx)
		delete(m.ctxs, reqid)
		m.resetTimer()
	}
	m.mu.Unlock()
	return ctx
}

func (m *contextManager) cleanExpired() {
	for range m.timer.C {
		for {
			var ctx *rpccontext
			m.mu.Lock()
			if len(m.wait) > 0 && time.Now().UnixNano() >= m.wait[0].expire {
				ctx = heap.Pop(m).(*rpccontext)
				delete(m.ctxs, ctx.id)
			}
			m.mu.Unlock()
			if ctx == nil {
				break
			}
			if ctx.done() {
				ctx.code = CodeTimeout
				ctx.result = nil
				m.c.chret <- ctx
			}
		}
		m.mu.Lock()
		m.resetTimer()
		m.mu.Unlock()
	}
}
