package micro

const (
	CodeOK       = 0
	CodeTimeout  = 1
	CodeFallback = 2
)

type ErrFunc func(action string, err error)

type Handler func(method string, params []byte) (int, []byte)

type Callback func(method string, code int, result []byte)
