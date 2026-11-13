#!/bin/bash
# A silver service with one endpoint and no contract file anywhere.
# The branch adds `currency` to what the endpoint returns.
set -eu
. "$(dirname "$0")/../install-qk.sh"
commit() { git add -A && git -c user.name=Eval -c user.email=eval@example.com commit -q -m "$1"; }

git init -q -b main
mkdir -p src
printf 'version: 1\nmaturity: silver\n' > quality.yaml
cat > src/orders.js <<'JS'
// GET /orders/{id}
export function getOrder(db, id) {
  const order = db.find(id);
  return { id: order.id, total: order.total };
}
JS
printf '# Orders\n\nA small service that returns orders.\n' > README.md
commit "Orders API"

git checkout -q -b order-currency
cat > src/orders.js <<'JS'
// GET /orders/{id}
export function getOrder(db, id) {
  const order = db.find(id);
  return { id: order.id, total: order.total, currency: order.currency };
}
JS
commit "Return the currency of an order"
