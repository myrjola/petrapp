-- Migration script to convert from blob user_id to integer user_id
-- This script should be run BEFORE the schema migration to ensure data integrity
-- 
-- Migration strategy:
-- 1. Add new integer id column and webauthn_user_id column to users table
-- 2. Populate integer IDs for existing users 
-- 3. Update all foreign key references to use integer user_id
-- 4. The new schema will then replace the old blob-based structure

-- Step 1: Add new columns to users table
ALTER TABLE users ADD COLUMN new_id INTEGER;
ALTER TABLE users ADD COLUMN webauthn_user_id BLOB;

-- Step 2: Populate new columns for existing users
-- Create integer IDs starting from 1 and store original blob ID as webauthn_user_id
UPDATE users SET 
    new_id = (SELECT COUNT(*) FROM users u2 WHERE u2.rowid <= users.rowid),
    webauthn_user_id = id;

-- Step 3: Add new columns to all tables that reference user_id
ALTER TABLE credentials ADD COLUMN new_user_id INTEGER;
ALTER TABLE workout_preferences ADD COLUMN new_user_id INTEGER;
ALTER TABLE workout_sessions ADD COLUMN new_user_id INTEGER;
ALTER TABLE workout_exercise ADD COLUMN new_workout_user_id INTEGER;
ALTER TABLE exercise_sets ADD COLUMN new_workout_user_id INTEGER;

-- Step 4: Populate new foreign key columns by looking up integer IDs
UPDATE credentials SET new_user_id = (
    SELECT new_id FROM users WHERE users.id = credentials.user_id
);

UPDATE workout_preferences SET new_user_id = (
    SELECT new_id FROM users WHERE users.id = workout_preferences.user_id
);

UPDATE workout_sessions SET new_user_id = (
    SELECT new_id FROM users WHERE users.id = workout_sessions.user_id
);

UPDATE workout_exercise SET new_workout_user_id = (
    SELECT new_id FROM users WHERE users.id = workout_exercise.workout_user_id
);

UPDATE exercise_sets SET new_workout_user_id = (
    SELECT new_id FROM users WHERE users.id = exercise_sets.workout_user_id
);

-- Verification queries (optional - for debugging):
-- SELECT 'users' as table_name, COUNT(*) as total, COUNT(new_id) as with_new_id, COUNT(webauthn_user_id) as with_webauthn_id FROM users
-- UNION ALL SELECT 'credentials', COUNT(*), COUNT(new_user_id), 0 FROM credentials
-- UNION ALL SELECT 'workout_preferences', COUNT(*), COUNT(new_user_id), 0 FROM workout_preferences
-- UNION ALL SELECT 'workout_sessions', COUNT(*), COUNT(new_user_id), 0 FROM workout_sessions
-- UNION ALL SELECT 'workout_exercise', COUNT(*), COUNT(new_workout_user_id), 0 FROM workout_exercise
-- UNION ALL SELECT 'exercise_sets', COUNT(*), COUNT(new_workout_user_id), 0 FROM exercise_sets;