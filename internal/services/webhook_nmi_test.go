package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransactionSubscriptionID_Fallbacks(t *testing.T) {
	tests := []struct {
		name string
		body *NMITransactionEventBody
		want string
	}{
		{
			name: "subscription reference",
			body: &NMITransactionEventBody{
				Subscription: &NMISubscriptionRef{SubscriptionID: " sub-123 "},
			},
			want: "sub-123",
		},
		{
			name: "transaction detail subscription",
			body: &NMITransactionEventBody{
				TransactionDetail: &NMITransactionDetail{
					Subscription: &NMISubscriptionRef{SubscriptionID: " detail-456 "},
				},
			},
			want: "detail-456",
		},
		{
			name: "order id fallback",
			body: &NMITransactionEventBody{OrderID: " order-789 "},
			want: "order-789",
		},
		{
			name: "transaction detail order id fallback",
			body: &NMITransactionEventBody{
				TransactionDetail: &NMITransactionDetail{OrderID: " detail-order "},
			},
			want: "detail-order",
		},
		{
			name: "po number fallback",
			body: &NMITransactionEventBody{PONumber: " po-001 "},
			want: "po-001",
		},
		{
			name: "transaction detail po number fallback",
			body: &NMITransactionEventBody{
				TransactionDetail: &NMITransactionDetail{PONumber: " detail-po "},
			},
			want: "detail-po",
		},
		{
			name: "customer id fallback",
			body: &NMITransactionEventBody{CustomerID: " cust-002 "},
			want: "cust-002",
		},
		{
			name: "transaction detail customer id fallback",
			body: &NMITransactionEventBody{
				TransactionDetail: &NMITransactionDetail{CustomerID: " detail-cust "},
			},
			want: "detail-cust",
		},
		{
			name: "empty payload",
			body: &NMITransactionEventBody{},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, transactionSubscriptionID(tc.body))
		})
	}
}

func TestTransactionActionSource(t *testing.T) {
	require.Equal(t, "recurring", transactionActionSource(&NMITransactionEventBody{
		Action: &NMIAction{Source: "Recurring"},
	}))

	require.Equal(t, "retry", transactionActionSource(&NMITransactionEventBody{
		TransactionDetail: &NMITransactionDetail{
			Action: &NMIAction{Source: "Retry"},
		},
	}))

	require.Equal(t, "", transactionActionSource(&NMITransactionEventBody{}))
}

func TestIsRecurringSource(t *testing.T) {
	require.True(t, isRecurringSource("recurring"))
	require.True(t, isRecurringSource("RETRY"))
	require.False(t, isRecurringSource("api"))
	require.False(t, isRecurringSource(""))
}
