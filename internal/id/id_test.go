// SPDX-License-Identifier: MIT

package id

import "testing"

func TestNew(t *testing.T) {
	seen := map[string]bool{}
	for range 10000 {
		got := New()
		if !Valid(got) {
			t.Fatalf("%q does not have the form of an ID", got)
		}
		if seen[got] {
			t.Fatalf("%q came twice in 10000", got)
		}
		seen[got] = true
	}
}

func TestValid(t *testing.T) {
	for s, want := range map[string]bool{"7k2m9q4x1c": true, "7K2M9Q4X1C": false, "7k2m9q4x1": false, "7k2m9q4x1co": false, "7k2m9q4xlc": false, "start": false} {
		if Valid(s) != want {
			t.Errorf("Valid(%q) != %v", s, want)
		}
	}
}
