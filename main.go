// Command invoicedupe scans an invoice line item export and flags line
// items that look like accidental duplicates: same customer, same amount,
// billed within a few days of each other. It answers one question -
// "did we maybe bill this twice?" - and nothing else.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// LineItem is one row from the input CSV.
type LineItem struct {
	InvoiceID   string
	LineID      string
	Customer    string
	Description string
	AmountCents int64
	Date        time.Time
}

// DupGroup is a set of line items that share a customer and amount and
// fall within the configured window of each other.
type DupGroup struct {
	Customer    string
	AmountCents int64
	Items       []LineItem
}

func main() {
	filePath := flag.String("file", "", "path to CSV file of invoice line items (required)")
	windowDays := flag.Int("window", 3, "max days apart for two line items with the same customer and amount to count as a possible duplicate")
	jsonOut := flag.Bool("json", false, "print results as JSON instead of a human-readable table")
	flag.Parse()

	if *filePath == "" {
		fmt.Fprintln(os.Stderr, "invoicedupe: -file is required")
		flag.Usage()
		os.Exit(2)
	}
	if *windowDays < 0 {
		fmt.Fprintln(os.Stderr, "invoicedupe: -window must not be negative")
		os.Exit(2)
	}

	items, err := readLineItems(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invoicedupe: %v\n", err)
		os.Exit(1)
	}

	groups := findDuplicates(items, *windowDays)

	if *jsonOut {
		printJSON(groups)
		return
	}
	printText(groups)
}

// readLineItems loads and validates the CSV at path. The header row is
// required and its column order is not assumed, so columns may appear in
// any order as long as all required names are present.
func readLineItems(path string) ([]LineItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}

	col := map[string]int{}
	for i, name := range header {
		col[strings.TrimSpace(strings.ToLower(name))] = i
	}
	for _, name := range []string{"invoice_id", "line_id", "customer", "description", "amount", "date"} {
		if _, ok := col[name]; !ok {
			return nil, fmt.Errorf("missing required column %q", name)
		}
	}

	var items []LineItem
	rowNum := 1
	for {
		rowNum++
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNum, err)
		}

		date, err := time.Parse("2006-01-02", strings.TrimSpace(record[col["date"]]))
		if err != nil {
			return nil, fmt.Errorf("row %d: invalid date: %w", rowNum, err)
		}
		cents, err := parseAmountCents(record[col["amount"]])
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNum, err)
		}

		items = append(items, LineItem{
			InvoiceID:   strings.TrimSpace(record[col["invoice_id"]]),
			LineID:      strings.TrimSpace(record[col["line_id"]]),
			Customer:    strings.TrimSpace(record[col["customer"]]),
			Description: strings.TrimSpace(record[col["description"]]),
			AmountCents: cents,
			Date:        date,
		})
	}
	return items, nil
}

// parseAmountCents turns a decimal dollar string like "129", "129.5" or
// "$129.00" into integer cents, so grouping never relies on float equality.
func parseAmountCents(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "$")

	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}

	parts := strings.SplitN(s, ".", 2)
	if parts[0] == "" {
		return 0, fmt.Errorf("invalid amount %q", raw)
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount %q: %w", raw, err)
	}

	var frac int64
	if len(parts) == 2 {
		fracStr := parts[1]
		if len(fracStr) == 0 || len(fracStr) > 2 {
			return 0, fmt.Errorf("invalid amount %q: expected at most 2 decimal digits", raw)
		}
		if len(fracStr) == 1 {
			fracStr += "0"
		}
		frac, err = strconv.ParseInt(fracStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid amount %q: %w", raw, err)
		}
	}

	cents := whole*100 + frac
	if neg {
		cents = -cents
	}
	return cents, nil
}

// findDuplicates buckets line items by customer and amount, then within
// each bucket chains together items that are no more than windowDays apart
// so that A-B-C billed a day apart each still cluster as one group even
// though A and C might be more than windowDays apart themselves.
func findDuplicates(items []LineItem, windowDays int) []DupGroup {
	buckets := map[string][]LineItem{}
	for _, it := range items {
		key := it.Customer + "|" + strconv.FormatInt(it.AmountCents, 10)
		buckets[key] = append(buckets[key], it)
	}

	window := time.Duration(windowDays) * 24 * time.Hour

	var groups []DupGroup
	for _, bucket := range buckets {
		sort.Slice(bucket, func(i, j int) bool { return bucket[i].Date.Before(bucket[j].Date) })

		var cluster []LineItem
		flush := func() {
			if len(cluster) > 1 {
				groups = append(groups, DupGroup{
					Customer:    cluster[0].Customer,
					AmountCents: cluster[0].AmountCents,
					Items:       cluster,
				})
			}
		}

		for i, it := range bucket {
			if i == 0 {
				cluster = []LineItem{it}
				continue
			}
			prev := cluster[len(cluster)-1]
			if it.Date.Sub(prev.Date) <= window {
				cluster = append(cluster, it)
				continue
			}
			flush()
			cluster = []LineItem{it}
		}
		flush()
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Customer != groups[j].Customer {
			return groups[i].Customer < groups[j].Customer
		}
		if groups[i].AmountCents != groups[j].AmountCents {
			return groups[i].AmountCents < groups[j].AmountCents
		}
		return groups[i].Items[0].Date.Before(groups[j].Items[0].Date)
	})
	return groups
}

func printText(groups []DupGroup) {
	if len(groups) == 0 {
		fmt.Println("no duplicate line items found")
		return
	}

	for _, g := range groups {
		fmt.Printf("%s  %s  %d possible duplicate line items\n", g.Customer, formatCents(g.AmountCents), len(g.Items))
		for _, it := range g.Items {
			fmt.Printf("  %s line %s  %s  %s\n", it.InvoiceID, it.LineID, it.Date.Format("2006-01-02"), it.Description)
		}
		fmt.Println()
	}

	if len(groups) == 1 {
		fmt.Println("1 possible duplicate group found")
	} else {
		fmt.Printf("%d possible duplicate groups found\n", len(groups))
	}
}

type jsonItem struct {
	InvoiceID   string `json:"invoice_id"`
	LineID      string `json:"line_id"`
	Date        string `json:"date"`
	Description string `json:"description"`
}

type jsonGroup struct {
	Customer    string     `json:"customer"`
	AmountCents int64      `json:"amount_cents"`
	Items       []jsonItem `json:"items"`
}

func printJSON(groups []DupGroup) {
	out := make([]jsonGroup, 0, len(groups))
	for _, g := range groups {
		jg := jsonGroup{
			Customer:    g.Customer,
			AmountCents: g.AmountCents,
			Items:       make([]jsonItem, 0, len(g.Items)),
		}
		for _, it := range g.Items {
			jg.Items = append(jg.Items, jsonItem{
				InvoiceID:   it.InvoiceID,
				LineID:      it.LineID,
				Date:        it.Date.Format("2006-01-02"),
				Description: it.Description,
			})
		}
		out = append(out, jg)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "invoicedupe: %v\n", err)
		os.Exit(1)
	}
}

// formatCents renders integer cents as a dollar string, e.g. 12900 -> "$129.00".
func formatCents(c int64) string {
	neg := c < 0
	if neg {
		c = -c
	}
	s := fmt.Sprintf("$%d.%02d", c/100, c%100)
	if neg {
		s = "-" + s
	}
	return s
}
