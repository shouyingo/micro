package micro

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync/atomic"

	"github.com/golang/protobuf/proto"
	"github.com/shouyingo/micro/microproto"
)

const (
	typeRequest  = 1
	typeResponse = 2
)

var (
	nconnid uint64
)

type conn struct {
	state  int32 // 0: alive, 1: down
	id     uint64
	c      net.Conn
	chrq   chan proto.Message
	chwq   chan proto.Message
	chdown chan struct{}
	err    error
}

func (c *conn) shutdown(err error) {
	if !atomic.CompareAndSwapInt32(&c.state, 0, 1) {
		return
	}
	c.err = err
	c.c.Close()
	close(c.chdown)
}

func (c *conn) readloop() {
	for {
		msg, err := c.read()
		if err != nil {
			c.shutdown(err)
			return
		}
		if isDebug {
			log.Printf("session(%d) read %s", c.id, msg)
		}
		if msg == nil {
			continue
		}
		select {
		case c.chrq <- msg:
		case <-c.chdown:
			return
		}
	}
}

func (c *conn) writeloop() {
	for {
		select {
		case msg := <-c.chwq:
			err := c.write(msg)
			if err != nil {
				c.shutdown(err)
				return
			}
			if isDebug {
				log.Printf("session(%d) write %s", c.id, msg)
			}
		case <-c.chdown:
			return
		}
	}
}

func (c *conn) read() (proto.Message, error) {
	var nbuf [4]byte
	_, err := io.ReadFull(c.c, nbuf[:])
	if err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(nbuf[:])
	if n < 4 || n >= maxPacketSize {
		return nil, fmt.Errorf("invalid packet size: %d", n)
	}
	if isDebug {
		log.Printf("session(%d) read n(%d)", c.id, n)
	}
	buf := make([]byte, n)
	_, err = io.ReadFull(c.c, buf)
	if err != nil {
		return nil, err
	}
	var msg proto.Message
	typ := binary.LittleEndian.Uint32(buf[:4])
	switch typ {
	case typeRequest:
		msg = &microproto.Request{}
	case typeResponse:
		msg = &microproto.Response{}
	default:
		return nil, fmt.Errorf("unsupported type: %d", typ)
	}
	err = proto.Unmarshal(buf[4:], msg)
	if err != nil {
		return nil, nil
	}
	return msg, nil
}

func (c *conn) write(msg proto.Message) error {
	var header [8]byte
	switch msg.(type) {
	case *microproto.Request:
		binary.LittleEndian.PutUint32(header[4:8], typeRequest)
	case *microproto.Response:
		binary.LittleEndian.PutUint32(header[4:8], typeResponse)
	default:
		return fmt.Errorf("unsupported type: %T", msg)
	}

	buf := proto.NewBuffer(header[:])
	err := buf.Marshal(msg)
	if err != nil {
		return err
	}
	data := buf.Bytes()
	binary.LittleEndian.PutUint32(data[0:4], uint32(len(data)-4))
	_, err = c.c.Write(data)
	return err
}

func (c *conn) send(msg proto.Message) bool {
	if atomic.LoadInt32(&c.state) == 0 {
		select {
		case c.chwq <- msg:
			return true
		case <-c.chdown:
		}
	}
	return false
}

func goconn(nc net.Conn) *conn {
	c := &conn{
		id:     atomic.AddUint64(&nconnid, 1),
		c:      nc,
		chrq:   make(chan proto.Message, connReadCap),
		chwq:   make(chan proto.Message, connWriteCap),
		chdown: make(chan struct{}),
	}
	go c.readloop()
	go c.writeloop()
	return c
}
