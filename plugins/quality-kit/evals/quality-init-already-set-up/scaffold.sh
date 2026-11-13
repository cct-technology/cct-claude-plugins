#!/bin/bash
# A silver repository with a quality.yaml and one finding: a dependency without go.sum (QK061).
set -eu
. "$(dirname "$0")/../install-qk.sh"
git init -q -b main
git remote add origin https://github.com/acme/billing.git
printf 'version: 1\nmaturity: silver\n' > quality.yaml
printf 'module github.com/acme/billing\n\ngo 1.24\n\nrequire github.com/google/uuid v1.6.0\n' > go.mod
printf 'package main\n\nfunc main() {}\n' > main.go
git add -A
git -c user.name=Eval -c user.email=eval@example.com commit -q -m "Billing"
