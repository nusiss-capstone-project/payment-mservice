package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewPaymentID(t *testing.T) {
	id, err := NewPaymentID()
	require.NoError(t, err)
	require.Len(t, id, PaymentIDLength)
	for _, c := range id {
		require.Contains(t, paymentIDCharset, string(c))
	}

	id2, err := NewPaymentID()
	require.NoError(t, err)
	require.NotEqual(t, id, id2)
}
