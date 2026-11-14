#!/bin/bash
# A small web app with one kind of user and no quality.yaml: a reading list
# behind a sign-in. Release 1.0.0 is tagged. Since then the list can be
# exported as a CSV file, with a unit test for the file's format.
set -eu
. "$(dirname "$0")/../install-qk.sh"
commit() { git add -A && git -c user.name=Eval -c user.email=eval@example.com commit -q -m "$1"; }

git init -q -b main
mkdir -p src public test
cat > README.md <<'MD'
# Shelf

A reading list. You sign in with your email address and keep a list of the
books you want to read. Everyone who signs in has their own list; there are no
other kinds of account.

A session ends after 30 minutes without activity.
MD
cat > src/server.js <<'JS'
import express from "express";
import { requireSignIn } from "./session.js";
import { books } from "./books.js";

const app = express();
app.use(express.static("public"));

// The list of the person who is signed in.
app.get("/books", requireSignIn, (req, res) => res.json(books.of(req.user.id)));
app.post("/books", requireSignIn, (req, res) => res.status(201).json(books.add(req.user.id, req.body)));

export default app;
JS
cat > src/session.js <<'JS'
// A session ends after 30 minutes without activity.
export const SESSION_MINUTES = 30;

export function requireSignIn(req, res, next) {
  const session = req.session;
  if (!session || session.idleMinutes() > SESSION_MINUTES) {
    return res.redirect("/sign-in");
  }
  req.user = session.user;
  next();
}
JS
cat > src/books.js <<'JS'
const lists = new Map();

export const books = {
  of(userId) {
    return lists.get(userId) ?? [];
  },
  add(userId, { title, author }) {
    const book = { title, author, added: new Date().toISOString().slice(0, 10) };
    lists.set(userId, [...this.of(userId), book]);
    return book;
  },
};
JS
cat > public/index.html <<'HTML'
<!doctype html>
<title>Shelf</title>
<h1>Your reading list</h1>
<form id="add">
  <input name="title" placeholder="Title" required>
  <input name="author" placeholder="Author">
  <button>Add book</button>
</form>
<ul id="books"></ul>
HTML
cat > CHANGELOG.md <<'MD'
# Changelog

## Unreleased

## 1.0.0

- Sign in and keep a reading list.
MD
commit "Shelf 1.0.0"
git tag v1.0.0

cat > src/export.js <<'JS'
// The list as a CSV file: a header line, then one line for each book.
// A value with a comma, a quote or a line break is put in quotes.
export function toCsv(list) {
  const cell = (v) => (/[",\n]/.test(v ?? "") ? `"${v.replaceAll('"', '""')}"` : (v ?? ""));
  return ["Title,Author,Added", ...list.map((b) => [b.title, b.author, b.added].map(cell).join(","))].join("\n") + "\n";
}
JS
cat > src/server.js <<'JS'
import express from "express";
import { requireSignIn } from "./session.js";
import { books } from "./books.js";
import { toCsv } from "./export.js";

const app = express();
app.use(express.static("public"));

// The list of the person who is signed in.
app.get("/books", requireSignIn, (req, res) => res.json(books.of(req.user.id)));
app.post("/books", requireSignIn, (req, res) => res.status(201).json(books.add(req.user.id, req.body)));

// The same list as a file to download.
app.get("/books/export.csv", requireSignIn, (req, res) => {
  res.attachment("reading-list.csv").type("text/csv").send(toCsv(books.of(req.user.id)));
});

export default app;
JS
cat > public/index.html <<'HTML'
<!doctype html>
<title>Shelf</title>
<h1>Your reading list</h1>
<form id="add">
  <input name="title" placeholder="Title" required>
  <input name="author" placeholder="Author">
  <button>Add book</button>
</form>
<a href="/books/export.csv" download>Download as CSV</a>
<ul id="books"></ul>
HTML
cat > test/export.test.js <<'JS'
import { test } from "node:test";
import assert from "node:assert/strict";
import { toCsv } from "../src/export.js";

test("a header line, then one line for each book", () => {
  assert.equal(toCsv([{ title: "Emma", author: "Jane Austen", added: "2026-01-05" }]), "Title,Author,Added\nEmma,Jane Austen,2026-01-05\n");
});

test("a value with a comma or a quote is put in quotes", () => {
  assert.equal(toCsv([{ title: 'Eats, Shoots & "Leaves"', author: "", added: "2026-01-06" }]), 'Title,Author,Added\n"Eats, Shoots & ""Leaves""",,2026-01-06\n');
});
JS
cat > CHANGELOG.md <<'MD'
# Changelog

## Unreleased

- Download your reading list as a CSV file, from the link under the form.

## 1.0.0

- Sign in and keep a reading list.
MD
commit "Download the reading list as CSV"
