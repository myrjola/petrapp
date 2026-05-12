You are about to hand off work to a fresh Claude session because this conversation's context is about to be wiped.

Output **exactly one fenced code block** and nothing else — no preamble, no commentary, no closing remarks. The code block contains a self-contained prompt the user will paste into a new session.

## What the handoff prompt must contain

1. **Cold-start framing.** State that the receiving session has zero context from this conversation and must rely on the cited files alone.

2. **Plan path.**
   - If `$ARGUMENTS` is non-empty, use it verbatim as the plan path.
   - Otherwise, infer the most recently written/edited plan under `docs/superpowers/plans/` from this conversation. If multiple are plausible, pick the one most recently authored or edited in this session. Cite the absolute repo-relative path.
   - If no plan can be inferred, emit the prompt with `<PLAN_PATH>` as a placeholder and a one-line note inside the code block telling the user to fill it in.

3. **Execution recipe**, in this order:
   1. Invoke `superpowers:using-git-worktrees` to create an isolated worktree for the feature branch derived from the plan filename (e.g. `feature/deload-periodization`).
   2. Inside that worktree, invoke `superpowers:subagent-driven-development` to execute the plan task-by-task with a fresh subagent per task and review checkpoints between them.
   3. When every task is complete and `make ci` is green, invoke `superpowers:finishing-a-development-branch` to decide between PR vs. direct merge, then merge the worktree branch back into `main` (prefer fast-forward; if not possible, a standard merge commit — never force-push).
   4. Clean up the worktree once the merge is complete.

4. **Safety guardrails.** Pause and confirm with the user before any destructive action: force push, `git reset --hard` on a branch with unique work, deleting untracked files, rebasing `main`, or skipping pre-commit hooks. Never use `--no-verify`.

5. **Reporting cadence.** After each task review checkpoint, post a one-line status to the user (task N of M complete, tests green/red). Do not wait until the end to report.

6. **Stop conditions.** Stop and ask the user if: a test fails for a reason unrelated to the current task, the plan's stated approach turns out to be wrong, `make ci` fails after a clean implementation, or the merge has conflicts.

## Formatting rules

- The handoff prompt is plain prose with bullet points where helpful. No nested code fences inside the outer code block — refer to skill names by their bare identifier (e.g. `superpowers:subagent-driven-development`).
- Keep the prompt under ~400 words. Sharp and complete beats exhaustive.
- Do NOT include this meta-instruction file's content in the output. The user only wants the receiving prompt.
