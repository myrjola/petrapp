# Exercise content cleanup — 2026-05-28

## Context

Companion to the generator fixes in
[spec](superpowers/specs/2026-05-27-exercise-data-quality-design.md) /
[plan](superpowers/plans/2026-05-27-exercise-data-quality.md). The new prompt
and Pass-2 URL validator (landed in `feat(service): drop rep guidance and
placeholders from exercise prompt` and the three commits after it) stop
new exercises from shipping with dead tutorial links or redundant rep
guidance in their descriptions. But the 39 exercises already in production
still have the old content. This script applies the same cleanup transforms
to those rows.

`internal/sqlite/fixtures.sql` was updated in the same commit so the seed
file (which is the source of truth for dev/test environments) stays in sync
with what prod will look like after this script runs.

## How the script was generated

```bash
make fly-backup FLY_APP=petra
fly ssh sftp get /data/snapshots/petrapp-petra-20260528T050634Z.sqlite3 \
    /tmp/petrapp-snapshots/prod-20260528.sqlite3 --app petra

go build -o /tmp/exercise-content-fixup ./cmd/exercise-content-fixup
/tmp/exercise-content-fixup \
    -db /tmp/petrapp-snapshots/prod-20260528.sqlite3 \
    -out docs/2026-05-28-exercise-content-cleanup.sql
```

`cmd/exercise-content-fixup` reads the snapshot, HEAD-checks every tutorial
URL referenced under `## Resources`, then writes a transactional `UPDATE`
SQL file with the cleaned descriptions. The tool is read-only against the
input DB; the SQL is the only deliverable.

## What this script changes

- **Dead tutorial links removed.** 91 unique URLs were HEAD-checked; ~40 of
  them now 403 or 404 (mostly verywellfit.com, physio-pedia.com, exrx.net,
  muscleandstrength.com, and a few barbend.com / self.com / bodybuilding.com
  guides that have moved). Their list items under `## Resources` are
  removed. YouTube tutorial URLs all returned 2xx and are kept.
- **`## Resources` headings removed when nothing survived.** When every list
  item in a Resources section was dead, the section heading is dropped too
  so the markdown doesn't render an empty section.
- **Rep-guidance lines removed from `## Instructions`.** Ordered-list items
  matching forms like `Perform 8-12 reps`, `Aim for 3 sets of 10-15 reps`,
  `Start with 3 sets of …`, `Hold this position for 30 seconds`, and the
  literal template leak `Repetition Guidance: …` are dropped. The app
  tracks rep and set targets separately and shows them to the user in the
  workout UI; these lines were duplicate information that went stale when
  users adjusted their rep window.

39 exercises were scanned; 34 modified. The 5 unchanged exercises (8, 10,
13, 18, 27) had no dead links and no rep-guidance lines.

## Run

```bash
make fly-sql-write SCRIPT=docs/2026-05-28-exercise-content-cleanup.sql FLY_APP=petra
```

The make target snapshots the live DB to `/data/snapshots/` before applying
the script (separate from the snapshot the script was generated against).
The whole script runs inside a single `BEGIN`/`COMMIT` transaction so a
partial failure leaves the DB untouched.

## Verification

After the script lands:

```bash
cat > /tmp/q.sql <<'SQL'
SELECT id, name,
       INSTR(description_markdown, 'example.com') AS has_placeholder,
       INSTR(description_markdown, '## Resources') AS has_resources_section
  FROM exercises ORDER BY id;
SQL
make fly-sql-readonly SCRIPT=/tmp/q.sql FLY_APP=petra
```

`has_placeholder` should be 0 for every row. `has_resources_section` should
be non-zero only for exercises whose Resources section has at least one
surviving live link.

Spot-check via the exercise-info page for a couple of known-affected
exercises (e.g. Pec Fly id=30, Hip Thrust id=35).
