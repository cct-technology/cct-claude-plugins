# Contracts: what to change and how

## The three kinds

| Kind | Describes | Key in `quality.yaml` | Compared by `qk` |
| --- | --- | --- | --- |
| OpenAPI | HTTP endpoints: paths, parameters, request and response bodies | `contracts.openapi` | Yes, QK030, when `oasdiff` is installed |
| AsyncAPI | Events and messages: channels and payloads | `contracts.asyncapi` | Not yet |
| ODCS (Open Data Contract Standard) | Datasets: tables, columns, types | `contracts.odcs` | Not yet |

## Editing a contract

- **Take every fact from the code.** A field's name, type and whether it is always present come from the handler, the serializer or the schema in the diff. When the code does not settle it, leave it out and say so in the report.
- **Follow the file's own style**: its indentation, whether schemas sit inline or under `components`, how it writes descriptions.
- **A field the code always returns** goes in `required`. One it may leave out does not.
- **A removed field** leaves `properties` and `required` both.
- **Leave `info.version`** as it is, unless the person asks for a new version.
- **Where the contract was already wrong** before this change, leave that part and name it in the report. Fixing it is a separate change with its own comparison.

## What breaks a client

Use this when no comparison ran, and to explain a finding.

Breaks:

- a response field, an endpoint, an event or a column that is removed or renamed
- a type that changes, or a value set that gets smaller
- a request field or parameter that is new and required, or was optional and is now required
- a response field that was always present and may now be missing

Does not break:

- a new response field, endpoint, event or column
- a new optional request field or parameter
- a request field that was required and is now optional

## Ways to stay compatible

Offer the ones that fit the change:

- **Keep the old field** beside the new one, and mark it `deprecated: true`.
- **Make the new requirement optional**, with a default.
- **Add a new version** of the endpoint or event, and leave the old one working.
- **Deprecate first, remove later**: mark it deprecated now, with the date it goes.

## The note for clients

One entry for each change a client can see:

```markdown
- `POST /invoices` now accepts an optional `due_date`. Without it the invoice is due in 30 days, as before. Clients need no change.
```

Say what changed, in the client's terms, and what a client has to do: nothing, or the step.

## Drafting a waiver for an intended break

Only after the person has confirmed the break is intended:

```sh
qk waiver add QK030 --path <contract file> \
  --reason "<why the break is acceptable: who the clients are and how they were told>" \
  --owner <who answers for it> --expires <YYYY-MM-DD> --approved-by <who approved>
```

- Ask for the owner, the approver and the date. Propose a date soon after the release that carries the break.
- A waiver accepts the finding for a time. It does not make the change compatible, so the note for clients still says what they must do.

## When there is no contract

Say which surface has none, for example "`GET /orders/{id}` has no OpenAPI file". To add one, the person writes or generates the file, then lists it:

```yaml
contracts:
  openapi: ["api/openapi.yaml"]
```

A contract generated from the code is the better start. Write one by hand only when the person asks for it.
