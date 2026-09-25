# invoicedupe

Billing systems export invoice line items in bulk, and accidental double
billing hides easily in that pile of rows: a webhook retries, someone
re-imports a batch, a recurring charge fires twice because a cron job
overlapped. Nobody catches this by eyeballing a spreadsheet.

`invoicedupe` answers one question: for a given export, which line items
look like they might be the same charge billed more than once? It flags
line items that share a customer and an exact amount and fall within a
few days of each other, and leaves the judgment call to you.

## Input format

A CSV file with a header row containing at least these columns, in any
order:

```
invoice_id,line_id,customer,description,amount,date
```

`amount` is a plain decimal dollar amount (`129`, `129.5`, `129.00`, an
optional leading `$` is fine). `date` is `YYYY-MM-DD`.

See `testdata/sample.csv` for a working example.

## Usage

```
go run . -file testdata/sample.csv
```

```
Acme Corp  $129.00  2 possible duplicate line items
  INV-1001 line 1  2026-09-01  Widget subscription - September
  INV-1004 line 1  2026-09-03  Widget subscription - September

1 possible duplicate group found
```

Widen the window to catch charges that repeated further apart:

```
go run . -file testdata/sample.csv -window 20
```

Machine-readable output for piping into another tool:

```
go run . -file testdata/sample.csv -json
```

```json
[
  {
    "customer": "Acme Corp",
    "amount_cents": 12900,
    "items": [
      {
        "invoice_id": "INV-1001",
        "line_id": "1",
        "date": "2026-09-01",
        "description": "Widget subscription - September"
      },
      {
        "invoice_id": "INV-1004",
        "line_id": "1",
        "date": "2026-09-03",
        "description": "Widget subscription - September"
      }
    ]
  }
]
```

## Flags

| flag       | default | meaning                                                         |
|------------|---------|------------------------------------------------------------------|
| `-file`    | -       | path to the CSV export (required)                                |
| `-window`  | `3`     | max days apart for a same customer/amount pair to count as a hit |
| `-json`    | `false` | print JSON instead of the table                                  |

## Why not just group by customer and amount with no window

Recurring charges are legitimate and land on the same customer/amount pair
every billing cycle. A tight day window is what separates "billed twice by
mistake" from "billed monthly on purpose."

## Status

Early. Built and tested against small CSV exports by hand so far.
