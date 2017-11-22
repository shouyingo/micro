package micro

const (
	CodeOK       = 0
	CodeTimeout  = 1
	CodeFallback = 2
)

type Logger interface {
	Printf(format string, args ...interface{})
}

type HandleFunc func(c *Context)

type Callback func(method string, code int, result []byte)

type noplogger struct{}

func (*noplogger) Printf(string, ...interface{}) {}

var anoplogger Logger = (*noplogger)(nil)
