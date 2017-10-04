package micro

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shouyingo/consul"
	"github.com/shouyingo/micro/microproto"
	"github.com/vizee/timer"
)

type rpccontext struct {
	state  int32 // 0: undone, 1: done
	c      *Client
	method string
	id     uint64
	fn     Callback
}

func (c *rpccontext) done() bool {
	return atomic.CompareAndSwapInt32(&c.state, 0, 1)
}

func (c *rpccontext) OnTime() {
	if c.done() {
		c.c.removeCtx(c.id)
		c.fn(c.method, CodeTimeout, nil)
	}
}

type serviceEntry struct {
	state int32 // 0: alive, 1: down
	id    string
	name  string
	addr  string
}

type Client struct {
	deps map[string]*conngroup // service => group

	nreqid  uint64
	ctxmu   sync.Mutex
	ctxs    map[uint64]*rpccontext // reqid => request
	timeout timer.Timer

	svcmu sync.RWMutex
	svcs  map[string]*serviceEntry // service-id => entry

	registry *consul.Client
	OnError  ErrFunc
}

func (c *Client) removeCtx(reqid uint64) *rpccontext {
	c.ctxmu.Lock()
	ctx := c.ctxs[reqid]
	if ctx != nil {
		delete(c.ctxs, reqid)
	}
	c.ctxmu.Unlock()
	return ctx
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
			case msg := <-server.chrq:
				resp, ok := msg.(*microproto.Response)
				if !ok {
					continue
				}
				ctx := c.removeCtx(resp.Id)
				if ctx != nil && ctx.done() {
					ctx.fn(ctx.method, int(resp.Code), resp.Result)
				}
			case <-server.chdown:
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
		err := c.registry.Watch(service, "", c.onWatch)
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
	ctx := &rpccontext{
		c:      c,
		method: method,
		id:     reqid,
		fn:     fn,
	}
	c.ctxmu.Lock()
	c.ctxs[reqid] = ctx
	c.ctxmu.Unlock()

	mc := g.randone()
	if mc == nil {
		c.removeCtx(reqid)
		return fmt.Errorf("service(%s) no alive session", service)
	}

	c.timeout.Add(ctx, time.Now().Add(rpcTimeout).UnixNano())
	ok := mc.send(&microproto.Request{
		Method: service + "." + method,
		Id:     reqid,
		Params: params,
	})
	if ok || !ctx.done() {
		return nil
	}
	c.removeCtx(reqid)
	return fmt.Errorf("service(%s) send request failed", service)
}

func NewClient(r *consul.Client, depends []string) *Client {
	deps := make(map[string]*conngroup, len(depends))
	for _, dep := range depends {
		deps[dep] = &conngroup{}
	}
	c := &Client{
		deps:     deps,
		ctxs:     make(map[uint64]*rpccontext),
		svcs:     make(map[string]*serviceEntry),
		registry: r,
	}
	for svc := range c.deps {
		go c.watchService(svc)
	}
	return c
}
