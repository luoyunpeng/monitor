package conf

import "time"

const (
	DefaultCollectDuration = 58 * time.Second
	DefaultCollectTimeOut  = DefaultCollectDuration + 10*time.Second
	DefaultMaxTimeoutTimes = 5
)
