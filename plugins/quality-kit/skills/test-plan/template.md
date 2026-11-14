<!-- Budget: two pages. At most 8 steps for a role, 3 charters and 6 security checks. One action and one expected result in a row, one sentence each. -->
# UAT plan: <release>

**Holds:** <the changes a person can see, one clause each>

**Before you start:** <the accounts, organisations and data a tester needs>

## How to record

When you have run a row, set its **Status** to `pass`, `fail` or `blocked`. `todo` means nobody has run it. Put what you saw under **Evidence**: a screenshot, a link to an issue, or a sentence. A row that is `fail` or `blocked` needs evidence. Write your name and the date under the table you ran.

## Steps for <role>

| ID | Step | Expected | Status | Evidence |
| --- | --- | --- | --- | --- |
| <ROLE>-01 | <one action, in the words on the screen> | <what the tester sees when it works> | todo | |

Run by: · Date:

## Exploratory charters

| ID | Charter | Time | Status | Evidence |
| --- | --- | --- | --- | --- |
| EXP-01 | Explore <an area> with <data or a habit> to find <a kind of problem> | 30 min | todo | |

Run by: · Date:

## Security acceptance checks

| ID | Check | Expected | Status | Evidence |
| --- | --- | --- | --- | --- |
| SEC-01 | Wrong role: as <a role without the right>, <try the action> | Refused, and nothing changed | todo | |
| SEC-02 | Another tenant: as <a role> of <one organisation>, open <a record of another> | Refused or not found, and nothing shown | todo | |
| SEC-03 | Expired session: <start the action>, wait <the session time>, then finish it | Sent to sign in, and nothing saved | todo | |

Run by: · Date:
