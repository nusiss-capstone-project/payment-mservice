package util

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const (
	PaymentIDLength  = 16
	paymentIDCharset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

// NewPaymentID returns a cryptographically random fixed-length base62 id.
func NewPaymentID() (string, error) {
	return randomBase62(PaymentIDLength)
}

func randomBase62(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("invalid length: %d", length)
	}
	max := big.NewInt(int64(len(paymentIDCharset)))
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate payment id: %w", err)
		}
		out[i] = paymentIDCharset[n.Int64()]
	}
	return string(out), nil
}
