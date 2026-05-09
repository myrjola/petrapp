-- Backfill rep_min/rep_max for production exercises not in fixtures.sql.
--
-- Context: the rep-scheme-derivation change adds a non-NULL CHECK on
-- (rep_min, rep_max) for every non-time_based row. Fixtures cover IDs
-- 1-21 and 24; production additionally holds IDs 22, 23, 25-31 (manually
-- backfilled exercises that aren't seeded). Without this backfill, the
-- next deploy would fail the CHECK on those rows.
--
-- Apply via fly-ops BEFORE deploying the schema change. After applying,
-- verify zero offenders with:
--
--   SELECT id, name FROM exercises
--   WHERE exercise_type <> 'time_based'
--     AND (rep_min IS NULL OR rep_max IS NULL);
--
-- Expected: zero rows.
--
-- Family classifications follow the table in
-- docs/superpowers/specs/2026-05-09-rep-scheme-derivation-design.md.

UPDATE exercises SET rep_min = 5,  rep_max = 10 WHERE id = 22; -- Incline Dumbbell Bench Press (compound, non-spinal)
UPDATE exercises SET rep_min = 8,  rep_max = 20 WHERE id = 23; -- Romanian Deadlift (lumbar-stress accessory)
UPDATE exercises SET rep_min = 8,  rep_max = 12 WHERE id = 25; -- Hip Abductor (isolation, large muscle)
UPDATE exercises SET rep_min = 8,  rep_max = 12 WHERE id = 26; -- Hip Adductor (isolation, large muscle)
UPDATE exercises SET rep_min = 8,  rep_max = 15 WHERE id = 27; -- Rotary Torso (stability / anti-extension)
UPDATE exercises SET rep_min = 10, rep_max = 20 WHERE id = 28; -- Seated Calf Raise (isolation, small/slow)
UPDATE exercises SET rep_min = 3,  rep_max = 6  WHERE id = 29; -- Squat (heavy spinal-load compound)
UPDATE exercises SET rep_min = 8,  rep_max = 12 WHERE id = 30; -- Pec Fly (isolation, large muscle)
UPDATE exercises SET rep_min = 3,  rep_max = 6  WHERE id = 31; -- Smith Machine Squat (heavy spinal-load compound)
