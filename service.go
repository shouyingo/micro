package micro

import (
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/shouyingo/consul"
	"github.com/shouyingo/micro/microproto"
)

type Service struct {
	state    int32 // 0: unstart, 1: started
	name     string
	registry *consul.Client
	handlers map[string]Handler
	OnError  ErrFunc
	svc      consul.AgentService
}

func (s *Service) register() error {
	id, err := s.registry.Register(&s.svc, serviceTTL+time.Second, serviceTimeout)
	if err != nil {
		return err
	}
	go s.keepalive(id)
	return nil
}

func (s *Service) keepalive(id string) {
	for {
		err := s.registry.KeepAlive(id, serviceTTL, nil)
		if err != nil {
			if eh := s.OnError; eh != nil {
				eh("keepalive", err)
			}
			break
		}
	}
retry:
	err := s.register()
	if err != nil {
		if eh := s.OnError; eh != nil {
			eh("register", err)
		}
		time.Sleep(time.Second)
		goto retry
	}
}

func (s *Service) handleClient(c net.Conn) {
	client := goconn(c)
	for {
		select {
		case msg := <-client.chrq:
			req, ok := msg.(*microproto.Request)
			if !ok {
				continue
			}
			fn := s.handlers[req.Method]
			if fn != nil {
				code, result := fn(req.Method[len(s.name)+1:], req.GetParams())
				client.send(&microproto.Response{
					Id:     req.Id,
					Code:   int32(code),
					Result: result,
					Ext:    req.Ext,
				})
			} else {
				client.send(&microproto.Response{
					Id:   req.Id,
					Code: CodeFallback,
					Ext:  req.Ext,
				})
			}
		case <-client.chdown:
			return
		}
	}
}

func (s *Service) Start() error {
	if !atomic.CompareAndSwapInt32(&s.state, 0, 1) {
		return fmt.Errorf("error state: %d", s.state)
	}

	l, err := net.Listen("tcp", net.JoinHostPort(s.svc.Address, strconv.Itoa(s.svc.Port)))
	if err != nil {
		return err
	}
	if s.svc.Port == 0 {
		_, p, _ := net.SplitHostPort(l.Addr().String())
		s.svc.Port, _ = strconv.Atoi(p)
	}
	err = s.register()
	if err != nil {
		l.Close()
		return err
	}
	for {
		nc, err := l.Accept()
		if err != nil {
			if eh := s.OnError; eh != nil {
				eh("accept", err)
			}
			time.Sleep(time.Second)
			continue
		}
		go s.handleClient(nc)
	}
}

var stubhanlder = map[string]Handler{}

func NewService(r *consul.Client, name string, addr string, handlers map[string]Handler) *Service {
	if len(handlers) == 0 {
		handlers = stubhanlder
	} else {
		h := make(map[string]Handler, len(handlers))
		for method, fn := range handlers {
			h[name+"."+method] = fn
		}
		handlers = h
	}
	h, p, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(p)
	return &Service{
		name:     name,
		registry: r,
		handlers: handlers,
		svc: consul.AgentService{
			Name:    name,
			Address: h,
			Port:    port,
			Tags:    []string{"micro"},
		},
	}
}
