package micro

import "time"

const (
	connWriteCap = 512
	connReadCap  = 256

	maxPacketSize = 64 * 1024

	serviceTTL     = 15 * time.Second
	serviceTimeout = time.Minute

	rpcTimeout = time.Second

	isDebug = false
)
