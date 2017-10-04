package micro

import "time"

const (
	connWriteCap = 1024
	connReadCap  = 512

	maxPacketSize = 64 * 1024

	serviceTTL     = 15 * time.Second
	serviceTimeout = time.Minute

	rpcTimeout = time.Second

	isDebug = false
)
