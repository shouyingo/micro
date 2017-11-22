package micro

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/shouyingo/consul"
)

type Service struct {
	state    int32 // 0: unstart, 1: started
	fn       HandleFunc
	registry *consul.Client
	logger   Logger
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
			s.logger.Printf("registry keepalive service(%s) failed: %s", id, err)
			break
		}
	}
retry:
	err := s.register()
	if err != nil {
		s.logger.Printf("registry register service(%s) failed: %s", s.svc.Name, err)
		time.Sleep(time.Second)
		goto retry
	}
}

func (s *Service) handleClient(nc net.Conn) {
	client := goconn(nc)
	for {
		select {
		case pack := <-client.chrq:
			if pack.Flag&FlagReply != 0 {
				continue
			}
			s.fn(&Context{
				id:     pack.Id,
				name:   pack.Name,
				params: pack.Body,
				ext:    pack.Ext,
				c:      client,
			})
		case <-client.chdown:
			if isDebug {
				log.Printf("client(%d) close: %s", client.id, client.err)
			}
			return
		}
	}
}

func (s *Service) SetLogger(logger Logger) {
	if logger != nil {
		s.logger = logger
	} else {
		s.logger = anoplogger
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
			s.logger.Printf("server(%s) accept failed: %s", s.svc.Name, err)
			time.Sleep(time.Second)
			continue
		}
		go s.handleClient(nc)
	}
}

func NewService(r *consul.Client, name string, addr string, fn HandleFunc) *Service {
	h, p, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(p)
	return &Service{
		registry: r,
		fn:       fn,
		logger:   anoplogger,
		svc: consul.AgentService{
			Name:    name,
			Address: h,
			Port:    port,
			Tags:    []string{"micro"},
		},
	}
}
