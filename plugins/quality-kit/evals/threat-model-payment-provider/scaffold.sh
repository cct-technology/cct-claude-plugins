#!/bin/bash
# A silver shop. The branch moves checkout to an external payment provider:
# the shop creates a payment session, the customer pays on the provider's
# page, and the provider calls a webhook. The handler does not check the
# event's signature yet.
set -eu
. "$(dirname "$0")/../install-qk.sh"
commit() { git add -A && git -c user.name=Eval -c user.email=eval@example.com commit -q -m "$1"; }

git init -q -b main
mkdir -p src docs/specs docs/security/threat-models
cat > quality.yaml <<'YAML'
version: 1
service: potter
data_class: client
maturity: silver
security:
  threat_models: docs/security/threat-models
  specs_dir: docs/specs
YAML
cat > docs/security/threat-models/README.md <<'MD'
# Threat models

One short threat model for each spec: what can go wrong, and what is done about it. Keep it to about two pages.
MD
cat > docs/specs/README.md <<'MD'
# Specs

One file for each feature, written before the code.
MD
cat > src/orders.js <<'JS'
import { Router } from "express";
import { requireCustomer } from "./auth.js";
import { db } from "./db.js";

export const orders = Router();

orders.post("/orders", requireCustomer, (req, res) => {
  res.status(201).json(db.orders.insert({ customerId: req.customer.id, lines: req.body.lines, status: "new" }));
});

orders.get("/orders/:id", requireCustomer, (req, res) => {
  const order = db.orders.find({ id: req.params.id, customerId: req.customer.id });
  if (!order) return res.status(404).end();
  res.json(order);
});
JS
cat > src/auth.js <<'JS'
export function requireCustomer(req, res, next) {
  if (!req.session?.customer) return res.status(401).end();
  req.customer = req.session.customer;
  next();
}

export function requireStaff(req, res, next) {
  if (!req.session?.staff) return res.status(401).end();
  req.staff = req.session.staff;
  next();
}
JS
commit "Potter: orders, paid by invoice"

git checkout -q -b checkout
cat > docs/specs/checkout.md <<'MD'
# Checkout

Customers pay for an order by card, through Paywell.

## What it does

1. The customer selects **Pay**. Potter asks Paywell for a payment session for the order's total, with our secret key.
2. The customer is sent to Paywell's page and pays there. Card numbers never reach Potter.
3. Paywell calls `POST /webhooks/paywell` with a signed event. On `payment.succeeded` Potter marks the order paid.
4. Staff can refund a paid order. Potter asks Paywell for the refund.

## Out of scope

- Storing cards.
- Subscriptions.
MD
cat > src/checkout.js <<'JS'
import { Router } from "express";
import { requireCustomer, requireStaff } from "./auth.js";
import { db } from "./db.js";

export const checkout = Router();
const PAYWELL = "https://api.paywell.example/v1";
const auth = { authorization: `Bearer ${process.env.PAYWELL_SECRET_KEY}`, "content-type": "application/json" };

// A payment session for the order, and the address of Paywell's page for it.
checkout.post("/orders/:id/pay", requireCustomer, async (req, res) => {
  const order = db.orders.find({ id: req.params.id, customerId: req.customer.id });
  if (!order) return res.status(404).end();
  const session = await fetch(`${PAYWELL}/sessions`, {
    method: "POST",
    headers: auth,
    body: JSON.stringify({ amount: order.total, currency: "NOK", reference: order.id, returnUrl: req.body.returnUrl }),
  }).then((r) => r.json());
  db.orders.update(order.id, { paywellSession: session.id, status: "awaiting payment" });
  res.json({ redirect: session.url });
});

checkout.post("/orders/:id/refund", requireStaff, async (req, res) => {
  const order = db.orders.find({ id: req.params.id, status: "paid" });
  if (!order) return res.status(404).end();
  await fetch(`${PAYWELL}/refunds`, { method: "POST", headers: auth, body: JSON.stringify({ session: order.paywellSession }) });
  res.json(db.orders.update(order.id, { status: "refunded", refundedBy: req.staff.id }));
});
JS
cat > src/webhooks.js <<'JS'
import { Router } from "express";
import { db } from "./db.js";

export const webhooks = Router();

// Paywell calls this when a payment changes. The event names our order in `reference`.
webhooks.post("/webhooks/paywell", (req, res) => {
  const event = req.body;
  if (event.type === "payment.succeeded") {
    db.orders.update(event.reference, { status: "paid" });
  }
  res.status(200).end();
});
JS
commit "Checkout through Paywell"
