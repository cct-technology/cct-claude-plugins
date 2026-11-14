---
description: A feature that sends ticket text to a hosted LLM. The threat model has the LLM section and goes through all ten OWASP risks, with a row or a reason for each.
tags: [threat-model]
max_turns: 40
timeout_seconds: 900
allowed_tools: [Read, Glob, Grep, Skill, Bash, Edit, Write, AskUserQuestion]
---

The reply-drafts feature sends ticket text to a hosted LLM. Do the threat model before we build more.
