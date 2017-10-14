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
	id    string
	name  string
	addr  string
}

type Client struct {
	deps   map[string]*conngroup // service => group
	mgr    *contextManager
	nreqid uint64

	svcmu sync.RWMutex
	svcs  map[string]*serviceEntry // service-id => entry

	registry *consul.Client
	OnError  ErrFunc
}

func (c *Client) handleServer(svc *serviceEntry) {
	g := c.deps[svc.name]
	if g == nil {
		return
	}
	for atomic.LoadInt32(&svc.state) == 0 {
		nc, err := net.Dial("tcp", svc.addr)
		if err != nil {
			if eh := c.OnError; eh != nil {
				eh("dial", err)
			}
			time.Sleep(time.Second)
			continue
		}
		server := goconn(nc)
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
					ctx.fn(ctx.method, ctx.code, ctx.result)
				}
			case <-server.chdown:
				if isDebug {
					log.Printf("server(%d) close: %s", server.id, server.err)
				}
				break mainloop
			}
		}
		g.remove(server)
	}
}

func (d *Client) onWatch(action int, id string, svc *consul.CatalogService) {
	switch action {
	case 0:
		s := &serviceEntry{
			id:   id,
			name: svc.ServiceName,
			addr: net.JoinHostPort(svc.ServiceAddress, strconv.Itoa(svc.ServicePort)),
		}
		d.svcmu.Lock()
		old := d.svcs[id]
		d.svcs[id] = s
		d.svcmu.Unlock()
		if old != nil {
			atomic.StoreInt32(&old.state, 1)
		}
		go d.handleServer(s)
	case 2:
		d.svcmu.Lock()
		s := d.svcs[id]
		if s != nil {
			atomic.StoreInt32(&s.state, 1)
			delete(d.svcs, id)
		}
		d.svcmu.Unlock()
	}
}

func (c *Client) watchService(service string) {
	for {
		err := c.registry.WatchCatalogService(service, "", c.onWatch)
		if err != nil {
			if eh := c.OnError; eh != nil {
				eh("watch", err)
			}
		}
		time.Sleep(time.Second)
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
		svcs:     make(map[string]*serviceEntry),
		registry: r,
	}
	c.mgr.c = c

	for svc := range c.deps {
		go c.watchService(svc)
	}
	go c.mgr.cleanExpired()
	return c
}
