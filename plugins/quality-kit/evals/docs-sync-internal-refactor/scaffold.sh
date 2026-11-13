#!/bin/bash
# A silver service with a how-to about totals. The branch refactors the code
# that works them out: same input, same result, nothing for a reader to notice.
set -eu
. "$(dirname "$0")/../install-qk.sh"
commit() { git add -A && git -c user.name=Eval -c user.email=eval@example.com commit -q -m "$1"; }

git init -q -b main
mkdir -p src docs/how-to
cat > quality.yaml <<'YAML'
version: 1
maturity: silver
docs:
  root: docs
  impact_map:
    - paths: ["src/**"]
      docs: ["docs/how-to/**"]
YAML
cat > src/price.js <<'JS'
// totalPrice returns what the items cost together, rounded to whole cents.
export function totalPrice(items) {
  let sum = 0;
  for (const item of items) {
    sum += item.price * item.quantity;
  }
  return Math.round(sum * 100) / 100;
}
JS
cat > docs/how-to/work-out-a-total.md <<'MD'
# Work out a total

Call `totalPrice` with the items of an order. Each item has a `price` and a `quantity`.

The total is the sum of price times quantity, rounded to whole cents.
MD
commit "Totals and their how-to"

git checkout -q -b tidy-price
cat > src/price.js <<'JS'
// lineTotal is what one line of an order costs.
function lineTotal(item) {
  return item.price * item.quantity;
}

// totalPrice returns what the items cost together, rounded to whole cents.
export function totalPrice(items) {
  const sum = items.reduce((total, item) => total + lineTotal(item), 0);
  return Math.round(sum * 100) / 100;
}
JS
commit "Split the line total out of totalPrice"
