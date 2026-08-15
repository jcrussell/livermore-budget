"""M0 spike, part 2 — prove the p66/p67 spine is mappable and self-checking.

Hand-maps the FY2025-26 revenue rows across all six fund groups and verifies
the parsed values against the totals the document itself prints.
"""
import pathlib
import re

SPIKE = pathlib.Path(__file__).parent / "spike" / "budget" / "pages"

# Row labels, in the order they appear on p66. p67 has NO labels -- it is a
# positional continuation, so this order is the row identity for both pages.
REVENUE_ROWS = [
    "Property Taxes", "Other Taxes", "Intergovernmental", "Charges for Services",
    "Use of Money And Property", "Contributions Outsourced", "Miscellaneous Revenue",
    "Sales Taxes", "Fines & Forfeitures", "Licenses & Permits",
]
EXPENDITURE_ROWS = ["Wages & Benefits", "Services & Supplies", "Capital Outlay", "Debt Services"]

# (fund_group, fiscal_year) in column order, per page.
P66_COLS = [("general", 2026), ("general", 2027), ("enterprise", 2026), ("enterprise", 2027)]
P67_COLS = [("capital", 2026), ("capital", 2027), ("debt-service", 2026), ("debt-service", 2027),
            ("special-revenue", 2026), ("special-revenue", 2027),
            ("internal-service", 2026), ("internal-service", 2027)]

# The strict amount grammar, in miniature. `-` is zero; `\-` is an escaped
# markdown dash, also zero. Parenthesised values are negative.
TOKEN = re.compile(r"\(?\$?-?[\d,]+\)?|\\-|-")


def parse_amount(tok):
    """Return integer cents. Raises on anything the grammar doesn't accept."""
    tok = tok.strip()
    if tok in ("-", "\\-", ""):
        return 0
    neg = tok.startswith("(") and tok.endswith(")")
    tok = tok.strip("()").lstrip("$")
    if " " in tok:
        raise ValueError(f"internal whitespace in amount: {tok!r}")
    if not re.fullmatch(r"[\d,]+", tok):
        raise ValueError(f"not an amount: {tok!r}")
    cents = int(tok.replace(",", "")) * 100
    return -cents if neg else cents


def numbers(text):
    return [parse_amount(t) for t in TOKEN.findall(text)]


def money(cents):
    return f"${cents / 100:,.2f}"


p66 = SPINE_66 = (SPIKE / "p0066.md").read_text()
p67 = (SPIKE / "p0067.md").read_text()

# --- p66: labelled rows, 4 columns each -------------------------------------
facts = []
for label in REVENUE_ROWS + EXPENDITURE_ROWS:
    m = re.search(re.escape(label) + r"\s+((?:(?:\(?\$?-?[\d,]+\)?|\\-|-)\s+){3}(?:\(?\$?-?[\d,]+\)?|\\-|-))", p66)
    assert m, f"row not found on p66: {label}"
    vals = numbers(m.group(1))
    assert len(vals) == 4, f"{label}: expected 4 values, got {len(vals)}: {vals}"
    kind = "revenue" if label in REVENUE_ROWS else "expenditure"
    for (group, fy), cents in zip(P66_COLS, vals):
        facts.append({"kind": kind, "row": label, "group": group, "fy": fy,
                      "cents": cents, "page": 66})

# --- checks against the document's own printed totals ------------------------
def stated(page_text, marker, n=4):
    m = re.search(re.escape(marker) + r"\s+((?:(?:\(?\$?-?[\d,]+\)?|\\-|-)\s+){%d}(?:\(?\$?-?[\d,]+\)?|\\-|-))" % (n - 1), page_text)
    assert m, f"marker not found: {marker}"
    return numbers(m.group(1))


checks = []
for i, (group, fy) in enumerate(P66_COLS):
    rev = sum(f["cents"] for f in facts if f["kind"] == "revenue" and f["group"] == group and f["fy"] == fy)
    exp = sum(f["cents"] for f in facts if f["kind"] == "expenditure" and f["group"] == group and f["fy"] == fy)
    checks.append((f"{group} FY{fy} revenues", rev, stated(p66, "TOTAL REVENUES:")[i]))
    checks.append((f"{group} FY{fy} expenditures", exp, stated(p66, "TOTAL EXPENDITURES:")[i]))

# --- p67: unlabelled positional continuation, 8 columns each -----------------
# p67 carries NO row labels; row identity comes from p66's order. It also OMITS
# rows that are zero across all four of its fund groups -- extraction collapses
# them. So the rule must declare the omission explicitly; a bare positional
# zip would silently shift every label after the gap.
OMITTED_ON_P67 = ["Licenses & Permits"]      # all-zero outside the General Fund
p67_rows = [r for r in REVENUE_ROWS if r not in OMITTED_ON_P67]

body = "\n".join(p67.splitlines()[4:])
body = body.split("$27,145,882")[0]          # stop at TOTAL REVENUES
vals = numbers(body)
assert len(vals) == len(p67_rows) * 8, (
    f"p67 row-count mismatch: {len(vals)} values is not {len(p67_rows)} rows x 8 columns. "
    f"Declared omissions: {OMITTED_ON_P67}")
for r, label in enumerate(p67_rows):
    for c, (group, fy) in enumerate(P67_COLS):
        facts.append({"kind": "revenue", "row": label, "group": group, "fy": fy,
                      "cents": vals[r * 8 + c], "page": 67})
for label in OMITTED_ON_P67:
    for group, fy in P67_COLS:
        facts.append({"kind": "revenue", "row": label, "group": group, "fy": fy,
                      "cents": 0, "page": 67})

p67_totals = stated(p67, "$27,145,882", n=8)   # the TOTAL REVENUES line itself
p67_totals = [parse_amount("27,145,882")] + p67_totals[:7]
for i, (group, fy) in enumerate(P67_COLS):
    rev = sum(f["cents"] for f in facts if f["kind"] == "revenue" and f["group"] == group and f["fy"] == fy)
    checks.append((f"{group} FY{fy} revenues", rev, p67_totals[i]))

print(f"{len(facts)} facts mapped from p66-p67\n")
ok = True
for name, got, want in checks:
    flag = "OK " if got == want else "FAIL"
    if got != want:
        ok = False
    print(f"  [{flag}] {name:<38} computed {money(got):>16}  stated {money(want):>16}")

print()
gf26 = sum(f["cents"] for f in facts if f["group"] == "general" and f["fy"] == 2026 and f["kind"] == "expenditure")
allrev = sum(f["cents"] for f in facts if f["kind"] == "revenue" and f["fy"] == 2026)
print(f"General Fund FY2025-26 expenditures : {money(gf26)}")
print(f"All-funds FY2025-26 revenues        : {money(allrev)}")
print(f"\n{'ALL CHECKS PASSED' if ok else 'CHECKS FAILED'}")
