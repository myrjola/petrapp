-- Demote 'Shoulders' from primary to secondary on three pressing/fly exercises.
-- See docs/2026-05-12-demote-shoulders-primary-on-presses.md for full context.
--
-- Why: Shoulders was being flagged "over" (red) on the home muscle-balance
-- bar while every other muscle was "under" (orange). Investigation showed
-- shoulders had exactly 10 primary sets (on target) but 13 secondary sets
-- piling on, pushing weighted load to 16.5 (>1.5x target).
--
-- Root cause: three chest-dominant exercises misclassify shoulders as a
-- PRIMARY muscle when shoulders is at most a synergist/stabilizer:
--   - Bench Press (id=2): had Chest+Shoulders+Triceps all primary.
--   - Incline Dumbbell Bench Press (id=22): had Chest+Shoulders primary.
--   - Pec Fly (id=30): had Chest+Shoulders primary. Pec Fly is a chest
--     isolation exercise; shoulders is purely stabilizing.
--
-- Scope of this script: shoulders only. Triceps stays primary on Bench
-- Press because triceps is a genuine prime mover on the press.
--
-- Bench Press (id=2) is also corrected in internal/sqlite/fixtures.sql so
-- the next deploy applies the same change via the fixture's ON CONFLICT
-- DO UPDATE clause. Pec Fly and Incline DBP are AI-generated and not in
-- the seed, so this one-shot is the only path to fix them in prod.
--
-- Match by exercise name (UNIQUE-constrained in the schema) so the script
-- is robust to any future id drift.
--
-- Run via:
--   make fly-sql-write SCRIPT=docs/2026-05-12-demote-shoulders-primary-on-presses.sql FLY_APP=petra

PRAGMA foreign_keys = ON;

BEGIN;

UPDATE exercise_muscle_groups
   SET is_primary = 0
 WHERE muscle_group_name = 'Shoulders'
   AND exercise_id IN (
       SELECT id FROM exercises
        WHERE name IN ('Bench Press', 'Incline Dumbbell Bench Press', 'Pec Fly')
   );

COMMIT;
