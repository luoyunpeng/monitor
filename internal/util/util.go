package util

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"math"
	"strings"
	"unsafe"
)

// Bytes2str use unsafe convert byte slice to string with zero allocate
func Bytes2str(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// Str2bytes use unsafe convert string to byte slice with zero allocate
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

// ComputeHmac256 encode with message and secret
func ComputeHmac256(message string, secret string) string {
	key := []byte(secret)
	h := hmac.New(sha512.New, key)
	h.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
