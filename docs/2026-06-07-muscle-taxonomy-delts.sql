-- One-shot prod cleanup for the side/rear-delt taxonomy split (Phase A).
-- fixtures.sql seeds the new desired state on every boot but does not delete
-- rows it no longer lists; this removes the obsolete production rows.
-- Apply once to production via the fly-ops path (make fly-sql-write SCRIPT=...).
-- Idempotent: re-running deletes nothing further.
--
-- Run via:
--   make fly-sql-write SCRIPT=docs/2026-06-07-muscle-taxonomy-delts.sql FLY_APP=petra

-- foreign_keys must be ON so the Hip Flexors delete cascades to its mappings.
PRAGMA foreign_keys = ON;

BEGIN;

-- Generic-shoulder rows that moved to a specific delt head.
DELETE FROM exercise_muscle_groups WHERE exercise_id = 5  AND muscle_group_name = 'Shoulders';
DELETE FROM exercise_muscle_groups WHERE exercise_id = 9  AND muscle_group_name = 'Shoulders';
DELETE FROM exercise_muscle_groups WHERE exercise_id = 34 AND muscle_group_name = 'Shoulders';

-- Drop the Hip Flexors group; ON DELETE CASCADE clears its exercise mappings
-- (Leg Extension, Plank, Hanging Leg Raise).
DELETE FROM muscle_groups WHERE name = 'Hip Flexors';

COMMIT;
