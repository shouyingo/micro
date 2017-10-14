package micro

import "sync/atomic"

type Context struct {
	id     uint64
	name   string
	params []byte
	ext    []byte
	state  int32
	c      *conn
}

func (c *Context) Method() string {
	return c.name
}

func (c *Context) Params() []byte {
	return c.params
}

func (c *Context) Reply(code int32, result []byte) bool {
	if !atomic.CompareAndSwapInt32(&c.state, 0, 1) {
		return false
	}
	return c.c.send(&Packet{
		Flag: FlagReply,
		Id:   c.id,
		Code: code,
		Body: result,
		Ext:  c.ext,
	})
}
