#!/bin/bash
# A Node.js service on GitHub that already has docs, and a CODEOWNERS at the root.
set -eu
. "$(dirname "$0")/../install-qk.sh"
git init -q -b main
git remote add origin https://github.com/acme/shop.git
mkdir -p docs/how-to src
printf '{\n  "name": "shop",\n  "version": "1.0.0",\n  "private": true\n}\n' > package.json
printf '# Shop\n\nTakes orders for the web shop. The guides are in docs/how-to.\n' > README.md
printf '# Our guides\n' > docs/how-to/README.md
printf '# Deploy\n\n1. Merge to main.\n2. Wait for the pipeline.\n' > docs/how-to/deploy.md
printf 'export const orders = [];\n' > src/orders.js
printf '* @acme/platform\n' > CODEOWNERS
git add -A
git -c user.name=Eval -c user.email=eval@example.com commit -q -m "Shop"
