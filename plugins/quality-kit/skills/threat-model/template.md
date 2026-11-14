---
spec: <the path of the spec from the repository root>
---
<!-- Budget: two pages. At most 5 boundaries and 12 threats, LLM threats among them. One sentence in a cell. Without a spec, leave the frontmatter out. -->
# Threat model: <feature>

**What it is:** <what the feature does, in one or two sentences>

**Data:** <the most sensitive data it handles>

**Uses an LLM:** <yes or no>

## Trust boundaries

| ID | Boundary | What crosses it | Code |
| --- | --- | --- | --- |
| B1 | <who or what is on each side> | <the data and the requests> | <the files where it is crossed> |

## Threats

| ID | Boundary | STRIDE | Threat | Mitigation | Status | Control |
| --- | --- | --- | --- | --- | --- | --- |
| T1 | B1 | <S, T, R, I, D or E> | <who does what, and what it costs> | <what stops it> | <in place or to do> | <A.x.y> |

`in place`: the code shows it today, in the file the mitigation names. `to do`: it does not yet.

## LLM threats

| ID | OWASP | Threat | Mitigation | Status | Control |
| --- | --- | --- | --- | --- | --- |
| L1 | <LLM01 to LLM10, with its name> | <who does what, and what it costs> | <what stops it> | <in place or to do> | <A.x.y> |

Does not apply: <each risk without a row, by its number, with the reason>

## Mitigations to do

- [ ] <T1>: <the work, in one sentence someone can pick up>

## Risks accepted

None. Only a person accepts a risk: write the threat's ID, why, your name and the date.
