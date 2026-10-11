---
title: "AI policy"
description: "Policy on AI-assisted contributions."
weight: 50
---

# AI policy

shaide welcomes the responsible use of AI tools. They can help contributors
explore ideas, write code, improve documentation, and review their work. The
person submitting a contribution remains responsible for everything included
in it.

This policy applies to external contributions, including pull requests,
issues, and discussions.

## Requirements

### Disclose AI assistance

State in the related PR whether an AI tool contributed to the submission. When it did, name the
tool and briefly describe how it was used and which parts of the work it
influenced.

A disclosure should use this format:

```text
AI assistance:
- Tool: <tool name>
- Model: <model name and version>
- Used for: <affected work>
- Human verification: <checks performed>
```

### Take ownership of the result

Before submitting AI-assisted work, you must be able to:

- explain the change without asking an AI tool to do so for you;
- describe how it interacts with the surrounding system;
- answer review questions and make follow-up changes

Do not submit generated code or text that you do not understand.

### Review and verify the work

Treat AI output as an untrusted draft. Check its technical claims, assumptions,
references, security implications, licensing, and compatibility with the
project. Run the relevant tests and other validation described in
`CONTRIBUTING.md`.

AI-assisted issues and discussions must be reviewed, fact-checked, and edited
by the person posting them. Keep submissions concise and remove irrelevant,
repetitive, speculative, or unsupported content.

### Meet the same quality standard

Using AI does not reduce the quality expected from a contribution. Maintainers
may close or reject work that is inaccurate, unverified, excessively noisy, or
places unreasonable validation work on reviewers.

Failure to disclose AI assistance, or repeated submission of low-quality
generated material, may result in contribution restrictions under the
project's moderation rules. These decisions are based on observable conduct
and contribution quality, not on a contributor's experience level.

New contributors are welcome to ask questions and seek guidance. AI should not
be used as a substitute for understanding or learning.

## Maintainer responsibility

This policy defines the requirements for external submissions. Maintainers may
use AI tools within the project's internal development process and remain
responsible for review, validation, and the quality of accepted changes.

## Why this policy exists

Every submission consumes the time and attention of human maintainers. The
disclosure and verification requirements help reviewers assess changes
efficiently and protect the project from plausible-looking but unreliable
output.

This is a quality and accountability policy, not a rejection of AI. shaide is
an AI platform, and AI-assisted work is welcome when a person understands,
checks, and stands behind the result.
