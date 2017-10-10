package micro

import (
	"log"
	"strconv"
	"sync"
	"testing"
	"time"
)

func printwait(m *contextManager) {
	for _, c := range m.wait {
		log.Println("method", c.method, "expire", c.expire, "hidex", c.hidx)
	}
	log.Println("====")
}

func TestContextManager(t *testing.T) {
	m := &contextManager{
		ctxs:  make(map[uint64]*rpccontext),
		timer: time.NewTimer(infinite),
	}
	go m.cleanExpired()
	wg := sync.WaitGroup{}
	fn := func(method string, code int, result []byte) {
		log.Println("done", method, code)
		wg.Done()
	}
	wg.Add(9)
	for i := 0; i < 4; i++ {
		meth := "test" + strconv.Itoa(i)
		log.Println("call", meth)
		m.add(uint64(i), meth, fn, time.Second)
	}
	ctx := m.add(4, "test4", fn, time.Second)
	for i := 9; i >= 5; i-- {
		meth := "test" + strconv.Itoa(i)
		log.Println("call", meth)
		m.add(uint64(i), meth, fn, time.Second)
	}
	printwait(m)
	log.Println("remove", 4)
	m.remove(ctx.id)
	printwait(m)

	wg.Wait()
	printwait(m)
}
