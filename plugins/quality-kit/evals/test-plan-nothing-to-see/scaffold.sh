#!/bin/bash
# A silver service whose next release changes nothing a person can see: a
# function is split in two, with the tests that cover it. Release 1.4.0 is tagged.
set -eu
. "$(dirname "$0")/../install-qk.sh"
commit() { git add -A && git -c user.name=Eval -c user.email=eval@example.com commit -q -m "$1"; }

git init -q -b main
mkdir -p src test docs/testing/uat
cat > quality.yaml <<'YAML'
version: 1
service: tariff
data_class: internal
maturity: silver
testing:
  uat_dir: docs/testing/uat
YAML
cat > docs/testing/uat/README.md <<'MD'
# UAT plans

One plan for each release, for the people who test it: the steps for each role, and where they record what they saw.
MD
cat > README.md <<'MD'
# Tariff

Works out what a parcel costs to send. Staff type the weight and the country
into the price page and read the price.
MD
cat > src/price.js <<'JS'
const RATES = { NO: 79, SE: 99, DK: 99 };

// The price of a parcel in NOK: the country's rate, plus 15 for each kilo over the first.
export function price(weightKg, country) {
  const rate = RATES[country];
  if (rate === undefined) throw new Error(`No rate for ${country}`);
  return rate + Math.max(0, Math.ceil(weightKg) - 1) * 15;
}
JS
cat > src/page.js <<'JS'
import { price } from "./price.js";

// The price page: weight and country in, price out.
export function show(form, out) {
  out.textContent = `${price(Number(form.weight.value), form.country.value)} NOK`;
}
JS
cat > test/price.test.js <<'JS'
import { test } from "node:test";
import assert from "node:assert/strict";
import { price } from "../src/price.js";

test("one kilo costs the country's rate", () => assert.equal(price(1, "NO"), 79));
test("each kilo over the first adds 15", () => assert.equal(price(2.2, "SE"), 129));
test("a country without a rate is refused", () => assert.throws(() => price(1, "FI")));
JS
cat > CHANGELOG.md <<'MD'
# Changelog

## Unreleased

## 1.4.0

- Prices for Denmark.
MD
commit "Tariff 1.4.0"
git tag v1.4.0

cat > src/price.js <<'JS'
const RATES = { NO: 79, SE: 99, DK: 99 };
const PER_EXTRA_KILO = 15;

// The rate of a country in NOK.
function rateOf(country) {
  const rate = RATES[country];
  if (rate === undefined) throw new Error(`No rate for ${country}`);
  return rate;
}

// The kilos that are charged on top of the rate: every started kilo over the first.
export function extraKilos(weightKg) {
  return Math.max(0, Math.ceil(weightKg) - 1);
}

// The price of a parcel in NOK: the country's rate, plus 15 for each kilo over the first.
export function price(weightKg, country) {
  return rateOf(country) + extraKilos(weightKg) * PER_EXTRA_KILO;
}
JS
cat > test/price.test.js <<'JS'
import { test } from "node:test";
import assert from "node:assert/strict";
import { extraKilos, price } from "../src/price.js";

test("one kilo costs the country's rate", () => assert.equal(price(1, "NO"), 79));
test("each kilo over the first adds 15", () => assert.equal(price(2.2, "SE"), 129));
test("a country without a rate is refused", () => assert.throws(() => price(1, "FI")));
test("a started kilo counts as a whole one", () => assert.equal(extraKilos(1.1), 1));
test("the first kilo is not extra", () => assert.equal(extraKilos(0.4), 0));
JS
cat > CHANGELOG.md <<'MD'
# Changelog

## Unreleased

- Internal: the price calculation is split into smaller functions, with more tests. Prices are the same.

## 1.4.0

- Prices for Denmark.
MD
commit "Split the price calculation into smaller functions"
