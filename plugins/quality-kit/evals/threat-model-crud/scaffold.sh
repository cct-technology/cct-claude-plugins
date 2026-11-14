#!/bin/bash
# A silver service with accounts in teams. The branch adds a spec for shared
# team notes and the code for it: plain create, read, update and delete, with
# no external service and no LLM. No threat model yet.
set -eu
. "$(dirname "$0")/../install-qk.sh"
commit() { git add -A && git -c user.name=Eval -c user.email=eval@example.com commit -q -m "$1"; }

git init -q -b main
mkdir -p src docs/specs docs/security/threat-models
cat > quality.yaml <<'YAML'
version: 1
service: huddle
data_class: internal
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
cat > src/auth.js <<'JS'
// A session ends after 30 minutes without activity. Every account belongs to one team.
export const SESSION_MINUTES = 30;

export function requireMember(req, res, next) {
  if (!req.session || req.session.idleMinutes() > SESSION_MINUTES) return res.status(401).end();
  req.account = req.session.account; // { id, teamId }
  next();
}
JS
cat > src/app.js <<'JS'
import express from "express";

export const app = express();
app.use(express.json({ limit: "100kb" }));
JS
commit "Huddle with accounts in teams"

git checkout -q -b team-notes
cat > docs/specs/team-notes.md <<'MD'
# Team notes

Members of a team keep notes that the whole team can read.

## What it does

- A member creates, edits and deletes notes in their own team.
- Every member of the team can read every note of the team. Nobody outside the team can.
- A note has a title and a body of up to 10 000 characters. The body is written in Markdown and shown as formatted text.
- Deleting a note is final.

## Out of scope

- Sharing a note with someone outside the team.
- A history of earlier versions.
MD
cat > src/notes.js <<'JS'
import { Router } from "express";
import { marked } from "marked";
import { requireMember } from "./auth.js";
import { db } from "./db.js";

export const notes = Router();

// Every query is scoped to the team of the account.
notes.get("/notes", requireMember, (req, res) => {
  res.json(db.notes.where({ teamId: req.account.teamId }));
});

notes.get("/notes/:id", requireMember, (req, res) => {
  const note = db.notes.find({ id: req.params.id, teamId: req.account.teamId });
  if (!note) return res.status(404).end();
  // The body as HTML, for the page to put in place.
  res.json({ ...note, html: marked.parse(note.body) });
});

notes.post("/notes", requireMember, (req, res) => {
  const { title, body } = req.body;
  if (!title || body.length > 10000) return res.status(422).end();
  res.status(201).json(db.notes.insert({ title, body, teamId: req.account.teamId, author: req.account.id }));
});

notes.put("/notes/:id", requireMember, (req, res) => {
  const note = db.notes.find({ id: req.params.id, teamId: req.account.teamId });
  if (!note) return res.status(404).end();
  res.json(db.notes.update(note.id, { title: req.body.title, body: req.body.body }));
});

notes.delete("/notes/:id", requireMember, (req, res) => {
  const note = db.notes.find({ id: req.params.id, teamId: req.account.teamId });
  if (!note) return res.status(404).end();
  db.notes.remove(note.id);
  res.status(204).end();
});
JS
commit "Team notes: the spec and the routes"
