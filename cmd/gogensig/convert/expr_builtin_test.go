package convert

import (
	"math"
	"testing"
)

func TestLitToUint(t *testing.T) {
	type CaseType struct {
		lit  string
		want uint64
	}

	int32Min := uint64(1) << 31
	testCases := []CaseType{
		{lit: "123", want: 123},
		{lit: "0xDEEDBEAF", want: 0xDEEDBEAF},                 // DEEDBEAF
		{lit: "0x80000000", want: int32Min},                   // INT32_MIN
		{lit: "0x7FFFFFFF", want: math.MaxInt32},              // INT32_MAX
		{lit: "0xFFFFFFFF", want: math.MaxUint32},             // UINT32_MAX
		{lit: "0xFFFFFFFFFFFFFFFF", want: 0xFFFFFFFFFFFFFFFF}, // UINT64_MAX
		{lit: "2147483647", want: math.MaxInt32},              // INT32_MAX
		{lit: "9223372036854775807", want: math.MaxInt64},     // INT64_MAX
	}
	for _, tc := range testCases {
		result, err := litToUint(tc.lit)

		if err != nil {
			t.Error(err)
		}

		if result^(tc.want) != 0 || result != (tc.want) {
			t.Error(tc.lit, "result mismatch want", tc.want, "got", result)
		}
	}
}

func TestLitToInt(t *testing.T) {
	type CaseType struct {
		lit  string
		want int64
	}

	testCases := []CaseType{
		{lit: "-2147483648", want: math.MinInt32},          // INT32_MIN
		{lit: "-9223372036854775808", want: math.MinInt64}, // INT64_MIN
	}
	for _, tc := range testCases {
		result, err := litToInt(tc.lit)

		if err != nil {
			t.Error(err)
		}

		if result^(tc.want) != 0 || result != (tc.want) {
			t.Error(tc.lit, "result mismatch want", tc.want, "got", result)
		}
	}
}
