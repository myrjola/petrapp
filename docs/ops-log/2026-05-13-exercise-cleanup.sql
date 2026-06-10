-- Clean up exercise muscle-group rows in prod that fixtures.sql can't fix.
-- See docs/2026-05-13-exercise-cleanup.md for full context.
--
-- Why: a data-quality audit (2026-05-13) surfaced three classes of issue:
--   1. Push-Up was credited with Biceps + Lats as PRIMARY in prod, when neither
--      is a prime mover (biceps has no elbow-flexion load on a push-up; lats
--      only stabilize the trunk). These extra rows came from an earlier seed
--      or manual edit and are not in the current fixtures.sql.
--   2. Bench Press had Upper Back as a SECONDARY, but the upper back acts as
--      an isometric stabilizer in scapular retraction, not as a worked muscle.
--   3. Pec Fly (AI-generated, not in fixtures) had Biceps and Triceps as
--      SECONDARIES, but both fly variants fix elbow angle so neither is
--      meaningfully loaded.
--
-- Companion fixtures.sql changes (same dev session):
--   - Push-Up muscle group rows expanded to include Forearms + Upper Back as
--     secondary, so fresh databases match the post-cleanup state.
--   - Bench Press 'Upper Back' row removed from the fixture's INSERT VALUES.
--   - Ab Wheel Rollout / Plank / Assisted Pull-Up muscle groupings rewritten
--     in the fixture; their corrections propagate via ON CONFLICT(exercise_id,
--     muscle_group_name) DO UPDATE — no rows in this script for those.
--   - 'One-Arm Dumbell Row' → 'One-Arm Dumbbell Row' and 'Push-up' → 'Push-Up'
--     renames happen via idempotent UPDATEs at the top of fixtures.sql, so
--     this script can assume the new names exist.
--
-- Match by exercise name (UNIQUE-constrained) so the script is robust to id
-- drift between environments.
--
-- Run via:
--   make fly-sql-write SCRIPT=docs/2026-05-13-exercise-cleanup.sql FLY_APP=petra
--
-- The make target snapshots the live DB to /data/snapshots/ before applying.

PRAGMA foreign_keys = ON;

BEGIN;

-- 1. Push-Up: drop Biceps (was primary) and Lats (was primary). The remaining
--    Push-Up muscle groups (Chest p, Triceps p, Shoulders s, Abs s, Forearms s,
--    Upper Back s) are already aligned with fixtures.sql after the deploy.
DELETE FROM exercise_muscle_groups
 WHERE muscle_group_name IN ('Biceps', 'Lats')
   AND exercise_id = (SELECT id FROM exercises WHERE name = 'Push-Up');

-- 2. Bench Press: drop Upper Back (was secondary).
DELETE FROM exercise_muscle_groups
 WHERE muscle_group_name = 'Upper Back'
   AND exercise_id = (SELECT id FROM exercises WHERE name = 'Bench Press');

-- 3. Pec Fly: drop Biceps and Triceps (both were secondary). Pec Fly stays
--    Chest primary + Shoulders secondary (Shoulders was already demoted by
--    docs/2026-05-12-demote-shoulders-primary-on-presses.sql).
DELETE FROM exercise_muscle_groups
 WHERE muscle_group_name IN ('Biceps', 'Triceps')
   AND exercise_id = (SELECT id FROM exercises WHERE name = 'Pec Fly');

COMMIT;
