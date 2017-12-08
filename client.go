package micro

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shouyingo/consul"
)

type serviceEntry struct {
	state int32 // 0: alive, 1: down
	mu    sync.Mutex
	c     *conn
	id    string
	name  string
	addr  string
}

func (s *serviceEntry) offline() {
	s.mu.Lock()
	atomic.StoreInt32(&s.state, 1)
	c := s.c
	s.c = nil
	s.mu.Unlock()
	if c != nil {
		c.shutdown(nil)
	}
}

type Client struct {
	deps   map[string]*conngroup // service => group
	mgr    *contextManager
	nreqid uint64

	registry *consul.Client
	logger   Logger
}

func (c *Client) handleServer(svc *serviceEntry) {
	g := c.deps[svc.name]
	if g == nil {
		return
	}
	for atomic.LoadInt32(&svc.state) == 0 {
		nc, err := net.Dial("tcp", svc.addr)
		if err != nil {
			c.logger.Printf("client dial service(%s) failed: %s", svc.name, err)
			time.Sleep(time.Second)
			continue
		}
		server := goconn(nc)

		svc.mu.Lock()
		if svc.state != 0 {
			svc.mu.Unlock()
			server.shutdown(nil)
			break
		}
		svc.c = server
		svc.mu.Unlock()

		g.add(server)
	mainloop:
		for {
			select {
			case pack := <-server.chrq:
				if pack.Flag&FlagReply == 0 {
					continue
				}
				ctx := c.mgr.remove(pack.Id)
				if ctx != nil && ctx.done() {
					ctx.fn(ctx.method, int(pack.Code), pack.Body)
				}
			case <-server.chdown:
				if isDebug {
					log.Printf("server(%d) close: %s", server.id, server.err)
				}
				break mainloop
			}
		}
		g.remove(server)
		time.Sleep(time.Second)
	}
}

func (c *Client) watchService(service string) {
	entries := map[string]*serviceEntry{}
	for {
		err := c.registry.WatchCatalogService(service, "", func(action int, id string, svc *consul.CatalogService) {
			switch action {
			case consul.WatchAdd, consul.WatchChange:
				e := entries[id]
				if e != nil {
					e.offline()
				}
				s := &serviceEntry{
					id:   id,
					name: svc.ServiceName,
					addr: net.JoinHostPort(svc.ServiceAddress, strconv.Itoa(svc.ServicePort)),
				}
				entries[id] = e
				go c.handleServer(s)
			case consul.WatchRemove:
				e := entries[id]
				if e != nil {
					delete(entries, id)
					e.offline()
				}
			}
		})
		if err != nil {
			c.logger.Printf("registry watch service(%s) failed: %s", service, err)
		}
		time.Sleep(time.Second)
	}
}

func (c *Client) SetLogger(logger Logger) {
	if logger != nil {
		c.logger = logger
	} else {
		c.logger = anoplogger
	}
}

func (c *Client) Call(service string, method string, params []byte, fn Callback) error {
	g := c.deps[service]
	if g == nil {
		return fmt.Errorf("%s not dependent service", service)
	}

	reqid := atomic.AddUint64(&c.nreqid, 1)
	ctx := c.mgr.add(reqid, method, fn, rpcTimeout)
	mc := g.randone()
	if mc == nil {
		if ctx.done() {
			c.mgr.remove(reqid)
			return fmt.Errorf("service(%s) no alive session", service)
		}
		return nil
	}

	ok := mc.send(&Packet{
		Id:   reqid,
		Name: method,
		Body: params,
	})
	if ok || !ctx.done() {
		return nil
	}
	c.mgr.remove(reqid)
	return fmt.Errorf("service(%s) send request failed", service)
}

func NewClient(r *consul.Client, depends []string) *Client {
	deps := make(map[string]*conngroup, len(depends))
	for _, dep := range depends {
		deps[dep] = &conngroup{}
	}

	c := &Client{
		deps: deps,
		mgr: &contextManager{
			ctxs:  make(map[uint64]*rpccontext),
			timer: time.NewTimer(timerIdle),
		},
		registry: r,
		logger:   anoplogger,
	}
	c.mgr.c = c

	for svc := range c.deps {
		go c.watchService(svc)
	}
	go c.mgr.cleanExpired()
	return c
}
