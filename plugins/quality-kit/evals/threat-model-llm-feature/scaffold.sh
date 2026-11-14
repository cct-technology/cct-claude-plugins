#!/bin/bash
# A support desk with no quality.yaml. The branch adds a spec and the first
# code for reply drafts: ticket text, the customer's earlier tickets and help
# articles found by vector search go to a hosted LLM, and an agent sends the
# draft after reading it.
set -eu
. "$(dirname "$0")/../install-qk.sh"
commit() { git add -A && git -c user.name=Eval -c user.email=eval@example.com commit -q -m "$1"; }

git init -q -b main
mkdir -p src docs/specs
cat > README.md <<'MD'
# Deskly

A support desk. Customers write in by email; agents answer from the ticket page.
MD
cat > src/tickets.js <<'JS'
import { Router } from "express";
import { requireAgent } from "./auth.js";
import { db } from "./db.js";

export const tickets = Router();

tickets.get("/tickets/:id", requireAgent, (req, res) => {
  const ticket = db.tickets.find({ id: req.params.id });
  if (!ticket) return res.status(404).end();
  res.json(ticket);
});

tickets.post("/tickets/:id/reply", requireAgent, (req, res) => {
  res.json(db.replies.insert({ ticketId: req.params.id, body: req.body.body, sentBy: req.agent.id }));
});
JS
cat > src/auth.js <<'JS'
export function requireAgent(req, res, next) {
  if (!req.session?.agent) return res.status(401).end();
  req.agent = req.session.agent;
  next();
}
JS
commit "Deskly: tickets and replies"

git checkout -q -b reply-drafts
cat > docs/specs/reply-drafts.md <<'MD'
# Reply drafts

When an agent opens a ticket, Deskly drafts a reply for them.

## What it does

- The draft is written by a hosted language model, Lumen, called over its API.
- Deskly sends Lumen: the customer's messages on the ticket, the customer's last five tickets, and the three help-centre articles that match best. The articles are found by vector search over embeddings of the help centre.
- The draft shows in the reply box. The agent reads it, changes it and sends it. Nothing is sent to a customer without an agent.
- Every draft is stored with its prompt, for quality review.

## Out of scope

- Sending a reply without an agent.
- Training a model on our tickets.
MD
cat > src/drafts.js <<'JS'
import { Router } from "express";
import { requireAgent } from "./auth.js";
import { db } from "./db.js";
import { nearestArticles } from "./search.js";

export const drafts = Router();

const SYSTEM = "You are a support agent for Deskly. Answer politely, from the articles given. Never promise a refund.";

drafts.post("/tickets/:id/draft", requireAgent, async (req, res) => {
  const ticket = db.tickets.find({ id: req.params.id });
  if (!ticket) return res.status(404).end();
  const earlier = db.tickets.where({ customerId: ticket.customerId }).slice(-5);
  const articles = await nearestArticles(ticket.messages.at(-1).body, 3);

  const prompt = [
    "Customer messages:", ...ticket.messages.map((m) => m.body),
    "Earlier tickets:", ...earlier.map((t) => t.subject + "\n" + t.messages.map((m) => m.body).join("\n")),
    "Articles:", ...articles.map((a) => a.body),
  ].join("\n\n");

  const answer = await fetch("https://api.lumen.example/v1/chat", {
    method: "POST",
    headers: { authorization: `Bearer ${process.env.LUMEN_API_KEY}`, "content-type": "application/json" },
    body: JSON.stringify({ model: "lumen-large", system: SYSTEM, messages: [{ role: "user", content: prompt }] }),
  }).then((r) => r.json());

  const draft = db.drafts.insert({ ticketId: ticket.id, prompt, body: answer.text, forAgent: req.agent.id });
  // The page puts the draft in the reply box as HTML.
  res.json({ id: draft.id, html: answer.text });
});
JS
cat > src/search.js <<'JS'
import { db } from "./db.js";

// The help-centre articles closest to the text, by the distance between embeddings.
export async function nearestArticles(text, count) {
  const vector = await embed(text);
  return db.articles.nearest(vector, count);
}

async function embed(text) {
  const r = await fetch("https://api.lumen.example/v1/embed", {
    method: "POST",
    headers: { authorization: `Bearer ${process.env.LUMEN_API_KEY}`, "content-type": "application/json" },
    body: JSON.stringify({ model: "lumen-embed", input: text }),
  }).then((r) => r.json());
  return r.vector;
}
JS
commit "Reply drafts: the spec and a first version"
