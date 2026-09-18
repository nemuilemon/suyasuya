package platform

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func Env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func Number(k string, fallback int64) int64 {
	v, e := strconv.ParseInt(os.Getenv(k), 10, 64)
	if e != nil || v <= 0 {
		return fallback
	}
	return v
}

func Token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}

func Digest(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }

func UUID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func ValidID(v string) bool {
	if len(v) != 36 {
		return false
	}
	_, e := hex.DecodeString(strings.ReplaceAll(v, "-", ""))
	return e == nil && len(strings.ReplaceAll(v, "-", "")) == 32
}

func SafeError(e error) string {
	s := e.Error()
	for _, k := range []string{"GEMINI_API_KEY", "DB_CONTROL_PASSWORD", "DB_AGENT_PASSWORD", "APP_PASSWORD"} {
		if v := os.Getenv(k); v != "" {
			s = strings.ReplaceAll(s, v, "[redacted]")
		}
	}
	return s
}
