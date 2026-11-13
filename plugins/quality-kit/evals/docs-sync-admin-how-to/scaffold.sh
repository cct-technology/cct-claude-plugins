#!/bin/bash
# A silver service with an admin how-to and a tutorial that both walk through
# the invite form. The branch adds a required role field to the form.
set -eu
. "$(dirname "$0")/../install-qk.sh"
commit() { git add -A && git -c user.name=Eval -c user.email=eval@example.com commit -q -m "$1"; }

git init -q -b main
mkdir -p web/admin/users docs/how-to/admin docs/tutorials
cat > quality.yaml <<'YAML'
version: 1
maturity: silver
docs:
  root: docs
  diataxis: true
  impact_map:
    - paths: ["web/admin/**"]
      docs: ["docs/how-to/admin/**"]
YAML
cat > web/admin/users/invite.html <<'HTML'
<h1>Invite a user</h1>
<form id="invite" action="/admin/users/invite" method="post">
  <label>Email <input name="email" type="email" required></label>
  <button type="submit">Send invite</button>
</form>
HTML
cat > docs/how-to/admin/invite-a-user.md <<'MD'
# Invite a user

1. Open **Users** in the admin menu.
2. Select **Invite**.
3. Enter the person's email address.
4. Select **Send invite**.

The person gets an email with a link that is valid for seven days.
MD
cat > docs/tutorials/first-day-as-admin.md <<'MD'
# Your first day as an admin

In this tutorial you sign in, invite a colleague and see them join.

## Sign in

1. Open the admin site and sign in with the account you were given.

## Invite a colleague

1. Open **Users** in the admin menu and select **Invite**.
2. Enter your colleague's email address and select **Send invite**.
3. Ask your colleague to open the email and follow the link.

## See them join

1. Open **Users** again. Your colleague is in the list.
MD
commit "Admin site with its docs"

git checkout -q -b invite-with-role
cat > web/admin/users/invite.html <<'HTML'
<h1>Invite a user</h1>
<form id="invite" action="/admin/users/invite" method="post">
  <label>Email <input name="email" type="email" required></label>
  <label>Role
    <select name="role" required>
      <option value="">Choose a role</option>
      <option value="member">Member</option>
      <option value="admin">Admin</option>
    </select>
  </label>
  <button type="submit">Send invite</button>
</form>
HTML
commit "Invite form asks for a role"
