package util

import (
	"math"
	"strings"
	"unsafe"

	"github.com/luoyunpeng/monitor/internal/config"
)

func Bytes2str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func Str2bytes(s string) []byte {
	x := (*[2]uintptr)(unsafe.Pointer(&s))
	h := [3]uintptr{x[0], x[1], x[1]}
	return *(*[]byte)(unsafe.Pointer(&h))
}

// Round return given the significant digit of float64
func Round(f float64, n int) float64 {
	pow10N := math.Pow10(n)
	return math.Trunc((f+0.5/pow10N)*pow10N) / pow10N
}

// IsInsideContainer check if monitor is running inside container
func IsInsideContainer() (bool, error) {
	lines, err := ReadLinesOffsetN("/proc/1/cgroup", 0, -1)
	if err != nil {
		return false, err
	}

	for _, line := range lines {
		if !strings.HasSuffix(line, "/") {
			return true, nil
		}
	}

	return false, nil
}

// CheckParam
func CheckParam(id, hostName string) string {
	if len(id) == 0 || len(hostName) == 0 {
		return "container id/name or host must given"
	}

	isHostKnown := false
	for _, h := range config.MonitorInfo.Hosts {
		if hostName == h {
			isHostKnown = true
		}
	}

	if !isHostKnown {
		return "nknown host, please try again"
	}
	return ""
}
