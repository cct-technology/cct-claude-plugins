#!/bin/bash
# A silver service with one endpoint and its OpenAPI contract.
# The branch stops returning `total`, which the contract promises: a break for clients.
set -eu
. "$(dirname "$0")/../install-qk.sh"
commit() { git add -A && git -c user.name=Eval -c user.email=eval@example.com commit -q -m "$1"; }

git init -q -b main
mkdir -p api src
cat > quality.yaml <<'YAML'
version: 1
maturity: silver
contracts:
  openapi: ["api/openapi.yaml"]
YAML
cat > api/openapi.yaml <<'YAML'
openapi: 3.0.3
info:
  title: Orders
  version: 1.0.0
paths:
  /orders/{id}:
    get:
      summary: Get one order
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: The order
          content:
            application/json:
              schema:
                type: object
                required: [id, total]
                properties:
                  id:
                    type: string
                  total:
                    type: number
                    description: What the order costs
YAML
cat > src/orders.js <<'JS'
// GET /orders/{id}
export function getOrder(db, id) {
  const order = db.find(id);
  return { id: order.id, total: order.total };
}
JS
cat > CHANGELOG.md <<'MD'
# Changelog

## Unreleased

## 1.0.0

- First release: `GET /orders/{id}`.
MD
commit "Orders API with its contract"

git checkout -q -b drop-total
cat > src/orders.js <<'JS'
// GET /orders/{id}
export function getOrder(db, id) {
  const order = db.find(id);
  return { id: order.id };
}
JS
commit "Stop returning the total of an order"
