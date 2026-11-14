# How-to guides

Each guide gets you to one goal. New here? Start with the [tutorial](../tutorials/first-run.md).

## Set up

- [Get the qk binary](get-the-qk-binary.md)
- [Set up Quality Kit with Claude](set-up-quality-kit-with-claude.md): the `quality-init` skill
- [Set up Quality Kit in a repository](set-up-quality-kit-in-a-repository.md): `qk init` on its own
- [Configure quality.yaml](configure-quality-yaml.md): maturity, and the mode of one check

## Day to day

- [Run qk check](run-qk-check.md)
- [Work with Quality Kit's hooks](work-with-the-hooks.md): what runs in a session, and when you are asked to approve
- [Waive a finding](waive-a-finding.md): an exception with a reason, an owner and an end date
- [Run Quality Kit in GitHub Actions](run-quality-kit-in-github-actions.md): the CI workflow, the pull request comment, SARIF

## With Claude, by skill

- [Update the docs for a change with Claude](update-docs-with-claude.md): `docs-sync`
- [Keep contracts true with Claude](keep-contracts-true-with-claude.md): `contract-guard`
- [Write a threat model with Claude](write-a-threat-model-with-claude.md): `threat-model`
- [Write a UAT plan with Claude](write-a-uat-plan-with-claude.md): `test-plan`

## By check

- QK010, QK020: [Keep docs in step with the code](keep-docs-in-step.md)
- QK030: [Catch breaking API changes](catch-breaking-api-changes.md)
- QK040: [Require a threat model for a new spec](require-a-threat-model-for-a-new-spec.md)
- QK060: [Check new and upgraded dependencies](check-dependencies.md)
- QK061: [Keep lockfiles in step with dependencies](keep-lockfiles-in-step.md)
- QK110: [Give every path a code owner](give-every-path-a-code-owner.md)

QK001 is covered by [Configure quality.yaml](configure-quality-yaml.md), and QK100 by [Waive a finding](waive-a-finding.md).

## Work on Quality Kit itself

- [Run the skill evals](run-the-skill-evals.md)
- [Release Quality Kit](release-quality-kit.md)
