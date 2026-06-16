# PetrApp — agent instructions

Orientation, development workflow, and the documentation index:
[README.md](README.md). Domain vocabulary: [CONTEXT.md](CONTEXT.md) — use those
canonical names; when code and CONTEXT.md disagree, reconcile the two rather
than letting them drift. Every layer has a `README.md` next to the code — read
it before working in that layer.

## Commands

```bash
make ci        # init + build + lint-fix + test — must pass before any push
make test      # go test --race ./... (cached, only changed packages re-run)
make lint-fix  # golangci-lint with --fix; run before committing
go test -v ./path/to/package -run TestName   # single test
```

Templates and static assets hot-reload from the filesystem — no rebuild while
iterating on UI. A govet shadow finding is usually fixed by reusing the
earlier `err` variable instead of introducing a new name.

## Shipping (trunk-based)

- Ship with `git push origin HEAD:main` once `make ci` passes — fix failures,
  never push around them.
- If the push is rejected because main moved: `git fetch origin && git rebase
  origin/main`, re-run `make ci`, push again. Never force-push.

## Worktrees

Multi-step design or implementation work happens in an isolated git worktree,
never the primary checkout.

- Remote sessions (`make claude-worktree-remote`) are **already** spawned into a
  fresh worktree under `.claude/worktrees/`, branched from origin/HEAD. Just
  work and ship — do not enter another worktree.
- Local sessions: enter one yourself (the EnterWorktree tool, or `claude
  --worktree <name>`) before the first written artifact.

Once `make ci` passes, ship with `git push origin HEAD:main`; the commits are
on origin/main from that point, so the worktree holds nothing you need to
preserve. Spawned remote worktrees are not removed when the session ends
(they're locked to the agent process), so they accumulate — prune them with
`git worktree remove --force` or a periodic sweep rather than relying on
session exit.

## Security

- Never introduce code that exposes or logs secrets and keys.
- Never commit secrets or keys to the repository.

## Agent skills

### Issue tracker

Issues and PRDs live as GitHub issues in `myrjola/petrapp`, managed via the
`gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Five canonical triage roles map to identically-named labels (`needs-triage`,
`needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See
`docs/agents/triage-labels.md`.

### Domain docs

Skill consumer rules — which ADRs to read before working an area, and how to
flag ADR conflicts — extend the CONTEXT.md guidance above. See
`docs/agents/domain.md`.
