# What to put in the plan

## What people test

A person judges what a test suite cannot: whether a task can be done from start to end, whether the screen says what it means, whether a message helps, whether the result is what someone doing this work would expect.

Leave to automation what it already covers. When a test in the repository checks a format, a sum or a rule, a row that has a person check it again costs their time and finds nothing. Say in the report which tests cover what you left out.

A row earns its place when:

- it is something the role does in their work, with the release's change in it
- the tester can tell from the screen whether it worked
- no automated test in the repository answers the same question

## Steps

- **One action and one expected result in a row.** "Select **Approve**" and "The invoice moves to **Sent**", not a paragraph.
- **The words on the screen.** Use the label, the menu name and the message as the code and docs write them. Mark a label in bold. For an action whose label you cannot find, say what the tester does in plain words, without bold.
- **The order of the work.** Start where the role starts, with the path they take most. Then the paths that end differently: a refusal, an empty list, a message.
- **Data a tester can make.** Say what to type when the value matters: a total over the limit, a title with a comma. Never data from a real customer.
- **Where two roles meet,** give each role its own rows and say in **Before you start** which comes first: the clerk submits before the approver can approve.

Give each row an ID: the role in capitals and a number, `CLERK-01`. An ID stays with its row for the life of the plan. A new row gets the next number; a removed row's number is not used again.

## Exploratory charters

A charter sends a tester off the path for a set time, with an aim:

> Explore <an area> with <data or a habit> to find <a kind of problem>.

Aim a charter at where the release meets what was there before, at data nobody planned for, and at work that is interrupted: a second tab, the back button, a lost connection. One charter can cover what several steps would only touch.

When `testing.replaces` in `quality.yaml` names a tool this system takes over from, give one charter to a task people did in that tool.

## Security acceptance checks

Three checks show that the release keeps people out of what is not theirs. Use the roles, organisations and session time you found, by name.

| Check | Applies when | What the tester does | Expected |
| --- | --- | --- | --- |
| Wrong role | The release adds or changes something only some roles may do | Signs in with a role that lacks the right, and tries: through the screen, and by opening the address directly | Refused, and nothing changed |
| Another tenant | Accounts belong to organisations, tenants or customers, and the release shows or changes their data | Signs in to one organisation and opens a record of another by its address or number | Refused or not found, and nothing of the other organisation shown |
| Expired session | The release is behind a sign-in | Starts the action, waits past the session time, then finishes it | Sent to sign in, and nothing saved or downloaded |

A check that does not apply gets one line under the table with the reason: "Wrong role: does not apply, every account has the same rights." The reader then knows it was considered.

Add a check of your own, up to the budget, when the release has a rule that keeps people apart which the three do not reach: nobody approves their own invoice.

## Before you start

Say what a tester needs in hand: an account for each role, a second organisation for the tenant check, the data the steps use. When `data_class` in `quality.yaml` is `client` or `internal`, say that testers use made-up data.

You do not know the address of the test system, who the testers are or when they test. Leave those to the person.

## Status

`todo`, `pass`, `fail` or `blocked`, in lower case, alone in the cell. `qk` will read this column at release time, so keep the table's columns and their order as the template has them.
