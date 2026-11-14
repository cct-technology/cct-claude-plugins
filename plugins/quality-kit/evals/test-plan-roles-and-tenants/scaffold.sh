#!/bin/bash
# A silver invoicing service with three roles, and organisations that must not
# see each other's invoices. Release 2.2.0 is tagged. Since then an invoice
# over the organisation's limit needs an approver before it is sent.
set -eu
. "$(dirname "$0")/../install-qk.sh"
commit() { git add -A && git -c user.name=Eval -c user.email=eval@example.com commit -q -m "$1"; }

git init -q -b main
mkdir -p src docs/testing/uat docs/reference
cat > quality.yaml <<'YAML'
version: 1
service: ledgerly
data_class: client
maturity: silver
testing:
  uat_dir: docs/testing/uat
YAML
cat > docs/testing/uat/README.md <<'MD'
# UAT plans

One plan for each release, for the people who test it: the steps for each role, and where they record what they saw.
MD
cat > docs/reference/roles.md <<'MD'
# Roles

Every account belongs to one organisation and has one role in it.

| Role | Can |
| --- | --- |
| `clerk` | Write invoices and send them |
| `approver` | Everything a clerk can |
| `admin` | Everything an approver can, and change the organisation's settings |

An account sees the invoices of its own organisation only. A session ends after 20 minutes without activity.
MD
cat > src/auth.js <<'JS'
export const ROLES = ["clerk", "approver", "admin"];
export const SESSION_MINUTES = 20;

// Refuses the request unless the account has one of the roles.
export const allow = (...roles) => (req, res, next) => {
  if (!req.session || req.session.idleMinutes() > SESSION_MINUTES) return res.status(401).end();
  if (!roles.includes(req.session.account.role)) return res.status(403).end();
  req.account = req.session.account;
  next();
};
JS
cat > src/invoices.js <<'JS'
import { Router } from "express";
import { allow } from "./auth.js";
import { db } from "./db.js";

export const invoices = Router();

// Every query is scoped to the organisation of the account.
invoices.get("/invoices", allow("clerk", "approver", "admin"), (req, res) => {
  res.json(db.invoices.where({ orgId: req.account.orgId }));
});

invoices.post("/invoices", allow("clerk", "approver", "admin"), (req, res) => {
  res.status(201).json(db.invoices.insert({ ...req.body, orgId: req.account.orgId, status: "draft" }));
});

invoices.post("/invoices/:id/send", allow("clerk", "approver", "admin"), (req, res) => {
  const invoice = db.invoices.find({ id: req.params.id, orgId: req.account.orgId });
  if (!invoice) return res.status(404).end();
  res.json(db.invoices.update(invoice.id, { status: "sent" }));
});
JS
cat > src/settings.js <<'JS'
import { Router } from "express";
import { allow } from "./auth.js";
import { db } from "./db.js";

export const settings = Router();

settings.get("/settings", allow("admin"), (req, res) => res.json(db.orgs.find({ id: req.account.orgId })));
JS
cat > CHANGELOG.md <<'MD'
# Changelog

## Unreleased

## 2.2.0

- Roles: clerk, approver and admin.
MD
commit "Ledgerly 2.2.0"
git tag v2.2.0

cat > src/invoices.js <<'JS'
import { Router } from "express";
import { allow } from "./auth.js";
import { db } from "./db.js";

export const invoices = Router();

// Every query is scoped to the organisation of the account.
invoices.get("/invoices", allow("clerk", "approver", "admin"), (req, res) => {
  res.json(db.invoices.where({ orgId: req.account.orgId }));
});

invoices.post("/invoices", allow("clerk", "approver", "admin"), (req, res) => {
  res.status(201).json(db.invoices.insert({ ...req.body, orgId: req.account.orgId, status: "draft" }));
});

// An invoice over the organisation's limit waits for an approver. The others are sent at once.
invoices.post("/invoices/:id/send", allow("clerk", "approver", "admin"), (req, res) => {
  const invoice = db.invoices.find({ id: req.params.id, orgId: req.account.orgId });
  if (!invoice) return res.status(404).end();
  const { approvalLimit } = db.orgs.find({ id: req.account.orgId });
  const status = approvalLimit != null && invoice.total > approvalLimit ? "awaiting approval" : "sent";
  res.json(db.invoices.update(invoice.id, { status, submittedBy: req.account.id }));
});

// The list an approver works from.
invoices.get("/approvals", allow("approver", "admin"), (req, res) => {
  res.json(db.invoices.where({ orgId: req.account.orgId, status: "awaiting approval" }));
});

// Nobody approves an invoice they submitted themselves.
invoices.post("/invoices/:id/approve", allow("approver", "admin"), (req, res) => {
  const invoice = db.invoices.find({ id: req.params.id, orgId: req.account.orgId, status: "awaiting approval" });
  if (!invoice) return res.status(404).end();
  if (invoice.submittedBy === req.account.id) return res.status(409).json({ error: "You submitted this invoice. Another approver has to approve it." });
  res.json(db.invoices.update(invoice.id, { status: "sent", approvedBy: req.account.id }));
});

// A rejected invoice goes back to draft with the reason, for the clerk to change.
invoices.post("/invoices/:id/reject", allow("approver", "admin"), (req, res) => {
  const invoice = db.invoices.find({ id: req.params.id, orgId: req.account.orgId, status: "awaiting approval" });
  if (!invoice) return res.status(404).end();
  if (!req.body.reason) return res.status(422).json({ error: "Give a reason." });
  res.json(db.invoices.update(invoice.id, { status: "draft", rejectedBecause: req.body.reason }));
});
JS
cat > src/settings.js <<'JS'
import { Router } from "express";
import { allow } from "./auth.js";
import { db } from "./db.js";

export const settings = Router();

settings.get("/settings", allow("admin"), (req, res) => res.json(db.orgs.find({ id: req.account.orgId })));

// The total over which an invoice needs approval. Empty means no invoice does.
settings.put("/settings/approval-limit", allow("admin"), (req, res) => {
  res.json(db.orgs.update(req.account.orgId, { approvalLimit: req.body.limit ?? null }));
});
JS
cat > docs/reference/roles.md <<'MD'
# Roles

Every account belongs to one organisation and has one role in it.

| Role | Can |
| --- | --- |
| `clerk` | Write invoices and send them. An invoice over the approval limit waits for an approver |
| `approver` | Everything a clerk can, and approve or reject the invoices that wait, except their own |
| `admin` | Everything an approver can, and change the organisation's settings, the approval limit among them |

An account sees the invoices of its own organisation only. A session ends after 20 minutes without activity.
MD
cat > CHANGELOG.md <<'MD'
# Changelog

## Unreleased

- An invoice over the organisation's approval limit waits for an approver before it is sent. Approvers find the invoices that wait under **Approvals**, and approve or reject them with a reason. Nobody approves their own invoice.
- Admins set the approval limit under **Settings**. Without a limit, invoices are sent as before.

## 2.2.0

- Roles: clerk, approver and admin.
MD
commit "Invoices over the limit need an approver"
