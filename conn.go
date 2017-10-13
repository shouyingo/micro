package micro

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync/atomic"
)

var (
	nconnid uint64
)

type conn struct {
	state  int32 // 0: alive, 1: down
	id     uint64
	c      net.Conn
	chrq   chan *Packet
	chwq   chan *Packet
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

func (c *conn) read() (*Packet, error) {
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
	pack := &Packet{}
	r := DecodePacket(pack, buf)
	if r == 0 || r == len(buf) {
		return nil, fmt.Errorf("invalid packet: %d", r)
	}
	return pack, nil
}

func (c *conn) write(pack *Packet) error {
	_, err := c.c.Write(EncodePacket(nil, pack))
	return err
}

func (c *conn) send(pack *Packet) bool {
	if atomic.LoadInt32(&c.state) == 0 {
		select {
		case c.chwq <- pack:
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
		chrq:   make(chan *Packet, connReadCap),
		chwq:   make(chan *Packet, connWriteCap),
		chdown: make(chan struct{}),
	}
	go c.readloop()
	go c.writeloop()
	return c
}
