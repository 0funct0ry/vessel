package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// Signature produces the value for X-Vessel-Signature.
func Signature(secret string, unix int64, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(strconv.FormatInt(unix, 10)))
	m.Write([]byte("."))
	m.Write(body)
	return "t=" + strconv.FormatInt(unix, 10) + ",v1=" + hex.EncodeToString(m.Sum(nil))
}
