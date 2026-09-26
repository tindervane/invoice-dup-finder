package main

import (
	"testing"
	"time"
)

func TestParseAmountCents(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"129", 12900},
		{"129.5", 12950},
		{"129.00", 12900},
		{"$129.00", 12900},
		{"0", 0},
		{"0.05", 5},
		{"-42.10", -4210},
		{"-$42.10", -4210},
		{"  129.00  ", 12900},
	}
	for _, c := range cases {
		got, err := parseAmountCents(c.in)
		if err != nil {
			t.Errorf("parseAmountCents(%q) returned error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseAmountCents(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseAmountCentsErrors(t *testing.T) {
	cases := []string{
		"",
		"$",
		"abc",
		"129.999",
		"129.",
		"-",
		"12.9.9",
	}
	for _, in := range cases {
		if _, err := parseAmountCents(in); err == nil {
			t.Errorf("parseAmountCents(%q) expected error, got none", in)
		}
	}
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("bad test date %q: %v", s, err)
	}
	return d
}

func TestFindDuplicatesWithinWindow(t *testing.T) {
	items := []LineItem{
		{InvoiceID: "INV-1", Customer: "Acme", AmountCents: 12900, Date: mustDate(t, "2026-09-01")},
		{InvoiceID: "INV-2", Customer: "Acme", AmountCents: 12900, Date: mustDate(t, "2026-09-03")},
	}
	groups := findDuplicates(items, 3)
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	if len(groups[0].Items) != 2 {
		t.Fatalf("got %d items in group, want 2", len(groups[0].Items))
	}
}

func TestFindDuplicatesOutsideWindow(t *testing.T) {
	items := []LineItem{
		{InvoiceID: "INV-1", Customer: "Northwind", AmountCents: 85000, Date: mustDate(t, "2026-09-05")},
		{InvoiceID: "INV-2", Customer: "Northwind", AmountCents: 85000, Date: mustDate(t, "2026-09-20")},
	}
	groups := findDuplicates(items, 3)
	if len(groups) != 0 {
		t.Fatalf("got %d groups, want 0 (items are outside the window)", len(groups))
	}
}

func TestFindDuplicatesDifferentCustomerOrAmountDoesNotGroup(t *testing.T) {
	items := []LineItem{
		{InvoiceID: "INV-1", Customer: "Acme", AmountCents: 12900, Date: mustDate(t, "2026-09-01")},
		{InvoiceID: "INV-2", Customer: "Globex", AmountCents: 12900, Date: mustDate(t, "2026-09-01")},
		{InvoiceID: "INV-3", Customer: "Acme", AmountCents: 6000, Date: mustDate(t, "2026-09-01")},
	}
	groups := findDuplicates(items, 3)
	if len(groups) != 0 {
		t.Fatalf("got %d groups, want 0", len(groups))
	}
}

func TestFindDuplicatesChainsAcrossWindow(t *testing.T) {
	// A-B are 2 days apart, B-C are 2 days apart, but A-C are 4 days apart,
	// which is beyond a window of 3. They should still cluster as one group
	// because clustering chains through consecutive neighbors.
	items := []LineItem{
		{InvoiceID: "INV-A", Customer: "Acme", AmountCents: 5000, Date: mustDate(t, "2026-09-01")},
		{InvoiceID: "INV-B", Customer: "Acme", AmountCents: 5000, Date: mustDate(t, "2026-09-03")},
		{InvoiceID: "INV-C", Customer: "Acme", AmountCents: 5000, Date: mustDate(t, "2026-09-05")},
	}
	groups := findDuplicates(items, 3)
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	if len(groups[0].Items) != 3 {
		t.Fatalf("got %d items in group, want 3", len(groups[0].Items))
	}
}

func TestFindDuplicatesNoMatches(t *testing.T) {
	items := []LineItem{
		{InvoiceID: "INV-1", Customer: "Acme", AmountCents: 12900, Date: mustDate(t, "2026-09-01")},
	}
	groups := findDuplicates(items, 3)
	if len(groups) != 0 {
		t.Fatalf("got %d groups, want 0 for a single line item", len(groups))
	}
}
