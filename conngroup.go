package micro

import (
	"math/rand"
	"sync"
)

type conngroup struct {
	mu sync.RWMutex
	ss []*conn
}

func (g *conngroup) randone() *conn {
	var s *conn
	g.mu.RLock()
	if len(g.ss) > 0 {
		s = g.ss[rand.Intn(len(g.ss))]
	}
	g.mu.RUnlock()
	return s
}

func (g *conngroup) add(s *conn) {
	g.mu.Lock()
	g.ss = append(g.ss, s)
	g.mu.Unlock()
}

func (g *conngroup) remove(s *conn) {
	g.mu.Lock()
	for i := 0; i < len(g.ss); i++ {
		if g.ss[i] == s {
			g.ss[i] = g.ss[len(g.ss)-1]
			g.ss = g.ss[:len(g.ss)-1]
			break
		}
	}
	g.mu.Unlock()
}
