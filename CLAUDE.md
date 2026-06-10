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

Any multi-step design or implementation task happens in an isolated git
worktree, not the primary checkout. Remote sessions via `make
claude-worktree-remote` already run in one; local sessions enter one (the
EnterWorktree tool, or `claude --worktree <name>`) before the first written
artifact. After shipping, end the session (or ExitWorktree with remove) — the
commits are already on origin/main.

## Security

- Never introduce code that exposes or logs secrets and keys.
- Never commit secrets or keys to the repository.
