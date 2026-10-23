// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package js

import "testing"

func TestNumberToString(t *testing.T) {
	for in, want := range map[float64]string{0: "0", 1e21: "1e+21", 1e20: "100000000000000000000", 1e-7: "1e-7",
		0.000001: "0.000001", 0.30000000000000004: "0.30000000000000004", -1.5e-9: "-1.5e-9", 123456789012345678901: "123456789012345680000"} {
		if got := NumberToString(in); got != want {
			t.Errorf("NumberToString(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestToFixed(t *testing.T) {
	for _, tc := range []struct {
		x      float64
		digits float64
		want   string
	}{{2.5, 0, "3"}, {1.005, 2, "1.00"}, {0.5, 0, "1"}, {-1.5, 0, "-2"}, {1e21, 2, "1e+21"}, {-0.0000001, 2, "-0.00"}} {
		if got, err := ToFixed(tc.x, tc.digits); err != nil || got != tc.want {
			t.Errorf("ToFixed(%v, %v) = %q, %v; want %q", tc.x, tc.digits, got, err, tc.want)
		}
	}
}

func TestStringToNumber(t *testing.T) {
	for in, want := range map[string]float64{"": 0, " 12 ": 12, "0x1f": 31, "0b101": 5, "1e3": 1000, ".5": 0.5} {
		if got := StringToNumber(in); got != want {
			t.Errorf("StringToNumber(%q) = %v, want %v", in, got, want)
		}
	}
	if got := StringToNumber("1_0"); got == got {
		t.Errorf("StringToNumber(1_0) = %v, want NaN", got)
	}
}

func TestCaseMapping(t *testing.T) {
	if got := ToUpperCase("straße"); got != "STRASSE" {
		t.Errorf("ToUpperCase = %q", got)
	}
	if got := ToLowerCase("ΟΔΟΣ ΑΣ."); got != "οδος ας." {
		t.Errorf("ToLowerCase = %q", got)
	}
}

func TestObjectKeyOrder(t *testing.T) {
	v, err := ParseJSON(`{"b":1,"2":2,"a":3,"1":4,"b":5,"01":6}`)
	if err != nil {
		t.Fatal(err)
	}
	s, _, _ := Stringify(v)
	if want := `{"1":4,"2":2,"b":5,"a":3,"01":6}`; s != want {
		t.Errorf("Stringify = %s, want %s", s, want)
	}
}
