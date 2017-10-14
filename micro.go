package micro

const (
	CodeOK       = 0
	CodeTimeout  = 1
	CodeFallback = 2
)

type ErrFunc func(action string, err error)

type HandleFunc func(c *Context)

type Callback func(method string, code int, result []byte)
