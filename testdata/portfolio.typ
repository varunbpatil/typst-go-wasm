#import "layout.typ": *

#let data = sys.inputs

// ── Section heading ────────────────────────────────────────────────────────
#let section-heading(title, first: false) = block(
  above: if first { 0pt } else { 14pt },
  below: 6pt,
  box(
    fill: primary,
    inset: (x: 7pt, y: 3pt),
    radius: 2pt,
    text(7pt, weight: "semibold", fill: white, tracking: 0.3pt, upper(title)),
  ),
)

// ── Gain/loss colored text ─────────────────────────────────────────────────
#let gain-text(v, body) = text(fill: if v >= 0 { gain-color } else { loss-color })[#body]

// ── Summary card ───────────────────────────────────────────────────────────
#let summary-card(label, value-body, sub: none) = block(
  fill: page-bg,
  inset: (x: 9pt, y: 8pt),
  radius: 3pt,
  width: 100%,
)[
  #text(5.5pt, fill: mid, upper(label))
  #v(3pt)
  #text(11pt, weight: "semibold")[#value-body]
  #if sub != none [
    #linebreak()
    #text(7pt)[#sub]
  ]
]

// ── Metric card ────────────────────────────────────────────────────────────
#let metric-card(label, value) = block(
  fill: page-bg,
  inset: (x: 8pt, y: 6pt),
  radius: 3pt,
  width: 100%,
)[
  #text(5.5pt, fill: mid, upper(label))
  #v(2pt)
  #text(9pt, weight: "semibold")[#value]
]

// ── Table helpers ──────────────────────────────────────────────────────────
#let tbl-hdr(..cols) = table.header(
  ..cols.pos().map(c => table.cell(fill: dark)[#text(fill: white, weight: "medium")[#c]]),
)

#let tbl-fill = (_, row) => if calc.odd(row) { row-alt } else { white }

// ── Document ───────────────────────────────────────────────────────────────
#show: apply-layout.with(data)

#let s = data.summary

// Portfolio Summary ──────────────────────────────────────────────────────────
#v(8pt)
#section-heading("Portfolio Summary", first: true)

#grid(
  columns: (1fr, 1fr, 1fr, 1fr),
  gutter: 8pt,
  summary-card("Total Value", fmt-money(s.total_value)),
  summary-card("Total Cost", fmt-money(s.total_cost)),
  summary-card(
    "Unrealized Gain",
    gain-text(s.unrealized_gain, fmt-money(s.unrealized_gain)),
    sub: gain-text(s.unrealized_gain_pct, fmt-pct(s.unrealized_gain_pct) + " return"),
  ),
  summary-card(
    "Day Change",
    gain-text(s.day_change, fmt-money(s.day_change)),
    sub: gain-text(s.day_change_pct, fmt-pct(s.day_change_pct)),
  ),
)

// Asset Allocation ───────────────────────────────────────────────────────────
#section-heading("Asset Allocation")

#table(
  columns: (1fr, auto, auto),
  align: (left, right, right),
  stroke: none,
  fill: tbl-fill,
  inset: (x: 6pt, y: 4pt),
  tbl-hdr("Asset Class", "Market Value", "Allocation"),
  ..data
    .asset_allocation
    .map(a => (
      a.name,
      fmt-money(a.value),
      fmt-pct(a.pct),
    ))
    .flatten(),
)

// Holdings ───────────────────────────────────────────────────────────────────
#section-heading("Holdings")

#table(
  columns: (auto, 1fr, auto, auto, auto, auto, auto),
  align: (left, left, right, right, right, right, right),
  stroke: none,
  fill: tbl-fill,
  inset: (x: 6pt, y: 4pt),
  tbl-hdr("Symbol", "Name", "Shares", "Price", "Market Value", "Cost Basis", "Gain / Loss"),
  ..data
    .holdings
    .map(h => (
      text(weight: "medium")[#h.symbol],
      text(fill: mid)[#h.name],
      fmt-num(h.shares),
      fmt-money(h.price),
      fmt-money(h.value),
      fmt-money(h.cost),
      gain-text(h.gain, fmt-money(h.gain) + " (" + fmt-pct(h.gain_pct) + ")"),
    ))
    .flatten(),
)

// Recent Transactions ────────────────────────────────────────────────────────
#section-heading("Recent Transactions")

#table(
  columns: (auto, auto, auto, 1fr, auto),
  align: (left, left, left, left, right),
  stroke: none,
  fill: tbl-fill,
  inset: (x: 6pt, y: 4pt),
  tbl-hdr("Date", "Type", "Symbol", "Description", "Amount"),
  ..data
    .transactions
    .map(tx => (
      tx.date,
      tx.type,
      tx.symbol,
      text(fill: mid)[#tx.description],
      gain-text(tx.amount, fmt-money(tx.amount)),
    ))
    .flatten(),
)

// Sector Breakdown & Key Metrics ─────────────────────────────────────────────
#v(14pt)
#grid(
  columns: (1fr, 1fr),
  gutter: 12pt,
  [
    #section-heading("Sector Breakdown", first: true)
    #table(
      columns: (1fr, auto, auto, auto),
      align: (left, right, right, right),
      stroke: none,
      fill: tbl-fill,
      inset: (x: 6pt, y: 4pt),
      tbl-hdr("Sector", "Market Value", "Allocation", "Positions"),
      ..data
        .sector_breakdown
        .map(s => (
          s.sector,
          fmt-money(s.value),
          fmt-pct(s.pct),
          str(s.positions),
        ))
        .flatten(),
    )
  ],
  [
    #section-heading("Key Metrics", first: true)
    #grid(
      columns: (1fr, 1fr),
      gutter: 6pt,
      ..data.metrics.map(m => metric-card(m.label, m.value)),
    )
  ],
)
