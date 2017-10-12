package micro

import "time"

const (
	connWriteCap = 512
	connReadCap  = 256

	clientRetCap = 1024

	maxPacketSize = 32 * 1024

	serviceTTL     = 15 * time.Second
	serviceTimeout = time.Minute

	rpcTimeout = time.Second

	isDebug = false
)
