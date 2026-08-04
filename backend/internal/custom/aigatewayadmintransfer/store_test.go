package aigatewayadmintransfer

import "testing"

// TestNormalizeAvailableBalance 验证零和负普通余额只会作为零可转余额返回，不能被误判为来源服务异常。
func TestNormalizeAvailableBalance(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
		valid bool
	}{
		{name: "zero", input: "0", want: "0.00000000", valid: true},
		{name: "negative balance", input: "-1.25", want: "0.00000000", valid: true},
		{name: "positive balance", input: "12.5", want: "12.50000000", valid: true},
		{name: "eight decimal places", input: "1.00000001", want: "1.00000001", valid: true},
		{name: "too precise", input: "1.000000001", valid: false},
		{name: "invalid", input: "not-a-number", valid: false},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			actual, valid := normalizeAvailableBalance(item.input)
			if valid != item.valid {
				t.Fatalf("normalizeAvailableBalance(%q) valid = %t, want %t", item.input, valid, item.valid)
			}
			if actual != item.want {
				t.Fatalf("normalizeAvailableBalance(%q) = %q, want %q", item.input, actual, item.want)
			}
		})
	}
}

// TestNormalizeAmount 验证来源扣款金额保留八位精度，并显式拒绝非正数和超过八位的小数。
func TestNormalizeAmount(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "eight decimal places", input: "12.50000001", want: "12.50000001"},
		{name: "pads trailing zeros", input: "12.5", want: "12.50000000"},
		{name: "zero", input: "0", want: ""},
		{name: "negative", input: "-1", want: ""},
		{name: "too precise", input: "1.000000001", want: ""},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if actual := normalizeAmount(item.input); actual != item.want {
				t.Fatalf("normalizeAmount(%q) = %q, want %q", item.input, actual, item.want)
			}
		})
	}
}
