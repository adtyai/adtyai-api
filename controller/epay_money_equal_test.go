package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEpayMoneyEqual(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want float64
		eq   bool
	}{
		{name: "exact two decimals", got: "10.00", want: 10, eq: true},
		{name: "integer string", got: "10", want: 10, eq: true},
		{name: "one decimal", got: "10.0", want: 10, eq: true},
		{name: "trailing zero", got: "10.000", want: 10, eq: true},
		{name: "spaces", got: " 10.00 ", want: 10, eq: true},
		{name: "mismatch", got: "10.01", want: 10, eq: false},
		{name: "empty", got: "", want: 10, eq: false},
		{name: "garbage", got: "abc", want: 10, eq: false},
		{name: "cents match", got: "0.1", want: 0.10, eq: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.eq, epayMoneyEqual(tc.got, tc.want))
		})
	}
}

func TestEpayParseMoneyCents(t *testing.T) {
	cents, err := epayParseMoneyCents("10")
	require.NoError(t, err)
	require.Equal(t, int64(1000), cents)

	cents, err = epayParseMoneyCents("10.00")
	require.NoError(t, err)
	require.Equal(t, int64(1000), cents)

	_, err = epayParseMoneyCents("")
	require.Error(t, err)
	_, err = epayParseMoneyCents("x")
	require.Error(t, err)
}
