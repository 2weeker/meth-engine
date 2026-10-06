package posterid

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"time"
)

const idBytes = 6

func Period(at time.Time) string { return at.UTC().Format("2006-01-02") }

func For(key []byte, ip string, at time.Time) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("meth-poster-id\x00"))
	mac.Write([]byte(ip))
	mac.Write([]byte{0})
	mac.Write([]byte(Period(at)))
	return base64.URLEncoding.EncodeToString(mac.Sum(nil)[:idBytes])
}
