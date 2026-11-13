#!/bin/bash
# A new Go service on GitHub with nothing but its module and an empty main.
set -eu
. "$(dirname "$0")/../install-qk.sh"
git init -q -b main
git remote add origin https://github.com/acme/orders.git
printf 'module github.com/acme/orders\n\ngo 1.24\n' > go.mod
printf 'package main\n\nfunc main() {}\n' > main.go
git add -A
git -c user.name=Eval -c user.email=eval@example.com commit -q -m "Start the orders service"
