# What to write for a change

## Would a reader notice?

A reader is anyone the docs serve: an end user, an admin, a developer who calls the API. A reader notices a change in what they do, see, type or get back:

- a setting, flag, command, endpoint, field, screen, button or step that is new, gone or renamed
- a different default, limit, format, error message or permission
- a step that moved, or that is now required

A reader notices nothing when the same input still gives the same result: code moved or renamed inside the system, a faster path, tests, comments, formatting.

When the diff does not settle it, treat the change as one a reader notices, and say in the report what you were unsure about. A missing doc costs the reader more than a spare sentence.

## How-to guide

The steps to reach one goal.

- Change the steps the diff changed, and leave the rest of the page as it is.
- Put a new step where the reader meets it, in the form the page already uses: its numbering, its way of marking labels.
- Use each name exactly as the code shows it: the label on the screen, the option in the list, the flag on the command.
- When no page covers the goal, write one in the folder the finding expects. Name it after the goal. Give it one sentence on the goal, numbered steps, and what the reader has at the end.

## Explanation

Why the system works the way it does. Update it when the change alters a reason, a trade-off or a design the page states. Leave it when only details moved.

## Reference

Facts to look up.

- **Generated:** run the command. Never edit the file.
- **Written by hand:** change only the facts the diff changed, copied from the code. Say in the report that the page has no generator, so the person can add one.

## Tutorial

A lesson someone follows step by step. Its steps are worth trusting only after someone has run them, and you cannot run them. So do not rewrite a tutorial: mark it.

Find tutorials under `tutorials/` in the docs folder (`docs.root` in `quality.yaml`, `docs` when it is not set). Search them for the names the change touches: labels, commands, settings, endpoints.

Put the mark directly under the tutorial's title:

```markdown
> **Re-run needed:** <what changed>. The steps below may no longer match.
```

A tutorial carries one mark. When it has one already, add what changed to its sentence.

## The no-docs note

The note speaks for every file on the branch. Propose it only when no finding is left that needs docs, and after the files that do need docs have them.

The reason says what changed and why a reader cannot tell:

- Good: `Moves the retry loop into withRetry; requests are sent and retried exactly as before.`
- Too thin: `Refactoring.` `No docs needed.`
