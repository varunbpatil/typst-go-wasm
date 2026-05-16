// Generic document layout utilities.

// ── Colours ────────────────────────────────────────────────────────────────
#let primary = rgb("#0d9488")   // teal-600
#let dark = rgb("#0f172a")   // slate-900
#let mid = rgb("#475569")   // slate-600
#let light = rgb("#94a3b8")   // slate-400
#let hdr-bg = rgb("#134e4a")   // teal-900
#let row-alt = rgb("#f8fafc")   // slate-50
#let page-bg = rgb("#f0fdfa")   // teal-50
#let gain-color = rgb("#059669")   // emerald-600
#let loss-color = rgb("#dc2626")   // red-600

// ── Number formatting ──────────────────────────────────────────────────────
#let fmt-money(v, decimals: 2) = {
  let negative = v < 0
  let abs-v = calc.abs(v)
  let factor = calc.pow(10, decimals)
  let rounded = calc.round(abs-v * factor) / factor
  let int-part = calc.floor(rounded)
  let frac-part = calc.round((rounded - int-part) * factor)
  let s = str(int-part)
  let result = ""
  let len = s.len()
  for i in range(len) {
    if i > 0 and calc.rem(len - i, 3) == 0 { result = result + "," }
    result = result + s.at(i)
  }
  let frac-str = str(frac-part)
  while frac-str.len() < decimals { frac-str = "0" + frac-str }
  if negative { "-$" + result + "." + frac-str } else { "$" + result + "." + frac-str }
}

#let fmt-num(v, decimals: 0) = {
  let negative = v < 0
  let abs-v = calc.abs(v)
  let factor = calc.pow(10, decimals)
  let rounded = calc.round(abs-v * factor) / factor
  let int-part = calc.floor(rounded)
  let frac-part = calc.round((rounded - int-part) * factor)
  let s = str(int-part)
  let result = ""
  let len = s.len()
  for i in range(len) {
    if i > 0 and calc.rem(len - i, 3) == 0 { result = result + "," }
    result = result + s.at(i)
  }
  if decimals == 0 { return if negative { "-" + result } else { result } }
  let frac-str = str(frac-part)
  while frac-str.len() < decimals { frac-str = "0" + frac-str }
  if negative { "-" + result + "." + frac-str } else { result + "." + frac-str }
}

#let fmt-pct(v, decimals: 2) = fmt-num(v, decimals: decimals) + "%"

// ── Header ─────────────────────────────────────────────────────────────────
#let conf-header(data) = {
  set text(font: "Rubik", size: 6pt, fill: white)
  block(width: 100%, fill: hdr-bg, inset: (x: 9mm, top: 5mm, bottom: 5mm), grid(
    columns: (1fr, auto),
    {
      text(size: 11pt, weight: "semibold", "Portfolio Report")
      linebreak()
      text(size: 6pt, fill: rgb("#99f6e4"), data.account_name + "  ·  Acct " + data.account_id)
    },
    align(right, {
      text(size: 6pt, fill: rgb("#99f6e4"), "Period")
      linebreak()
      text(size: 7pt, weight: "medium", data.period_start + " – " + data.period_end)
    }),
  ))
}

// ── Footer ─────────────────────────────────────────────────────────────────
#let footer-height = 16pt

#let conf-footer = block(
  width: 100%,
  height: footer-height,
  stroke: (top: 0.5pt + rgb("#e2e8f0")),
  inset: (x: 5mm, y: 0pt),
  align(horizon, text(5.5pt, fill: light, "For informational purposes only. Not financial advice.")),
)

// ── Page setup template ────────────────────────────────────────────────────
#let apply-layout(data, body) = {
  set page(
    paper: "us-letter",
    margin: (top: 42pt, bottom: footer-height, x: 0pt),
    header: conf-header(data),
    footer: conf-footer,
    header-ascent: 0pt,
    footer-descent: 0pt,
  )
  set text(font: "Rubik", size: 7pt, fill: dark, weight: "regular", features: ("tnum",))
  set par(leading: 0.55em)
  pad(x: 12mm, body)
}
