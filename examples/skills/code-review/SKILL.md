---
name: code-review
description: Review a change for correctness and production risk
max_steps: 20
tools:
  - get_diff
  - get_file
  - search_code
---
# Code Review

1. Read the complete diff.
2. Identify changed symbols and search for their callers.
3. Read enough surrounding code to verify behavior.
4. Report only actionable correctness, security, concurrency, data-loss, or compatibility problems.
5. Cite the affected file and line in every finding.
