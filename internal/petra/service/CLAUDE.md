# Service — agent notes

Package reference: [README.md](README.md).

- Never mock the repository layer in tests — service tests run against a real
  in-memory SQLite via `setupTestService`.
- Mutations go through `WeekPlans.Update` closures calling aggregate methods;
  wrap errors with `fmt.Errorf` so domain sentinels still match `errors.Is`.
