package model

import "testing"

func TestFormatMoney(t *testing.T) {
	if FormatMoney(2899) != "28.99" {
		t.Fatalf("got %s", FormatMoney(2899))
	}
	if FormatMoney(-50) != "-0.50" {
		t.Fatalf("got %s", FormatMoney(-50))
	}
	if FormatMoney(0) != "0.00" {
		t.Fatalf("got %s", FormatMoney(0))
	}
}
