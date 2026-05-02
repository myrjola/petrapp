-- Recovery for the production exercise clobbered by commit 9f6fc8e
-- ("feat(fixtures): add Assisted Pull-Up exercise") on 2026-05-01.
--
-- Source for the Incline Dumbbell Bench Press row + muscle groups:
--   litestream restore -timestamp 2026-05-01T11:00:00Z of /data/petrapp.sqlite3
--   on the petra Fly app, queried 2026-05-02.
--
-- Original cause: fixtures.sql used ON CONFLICT(id) DO UPDATE SET name = ...
-- which silently overwrote the manually-inserted Incline Dumbbell Bench Press
-- row at id=22 when the new fixture introduced a different exercise at the
-- same id. That ON CONFLICT clause has since been changed to key on name,
-- and Assisted Pull-Up has been moved to id=24 in fixtures.sql to free id=22
-- for the recovered row.
--
-- This script:
--   1. Rewrites id=22 in place from Assisted Pull-Up back to Incline Dumbbell
--      Bench Press. id=22 is preserved (not reassigned) because production
--      already has user workout history at exercise_id=22 that should stay
--      attached to Incline DBP, which is what id=22 always meant pre-bug.
--   2. Recreates Assisted Pull-Up at id=24, matching the updated fixtures.sql.
--      Users who picked AP between the buggy deploy and this recovery will
--      have their workout_exercise rows silently re-attributed to Incline DBP;
--      that is accepted as part of restoring the pre-bug intent of id=22.
--
-- Run via:
--   make fly-sql-write SCRIPT=docs/2026-05-02-recover-incline-dumbbell-bench-press.sql FLY_APP=petra

PRAGMA foreign_keys = ON;

BEGIN;

UPDATE exercises
   SET name                 = 'Incline Dumbbell Bench Press',
       category             = 'upper',
       exercise_type        = 'weighted',
       description_markdown = '## Instructions
1. **Bench Setup**: Adjust a bench to an incline of 30-45 degrees. Lie back on the bench while holding a dumbbell in each hand resting just above your chest.
2. **Foot Positioning**: Firmly plant your feet on the floor to provide stability through your legs and lower back. Your shoulder blades should be pulled back for support.
3. **Pressing Motion**: As you exhale, press the dumbbells upward until your arms are fully extended but not locked out.
4. **Return Movement**: Inhale as you slowly lower the dumbbells back to the starting position, keeping your elbows slightly below the shoulders.
5. **Repetition Guidance**: Aim for 8-12 repetitions, focusing on controlled movements.

## Common Mistakes
- **Arching Back**: Avoid excessive arching of the back by engaging your core and keeping your back flat against the bench.
- **Flared Elbows**: Tucking your elbows too far out can strain the shoulders. Keep them at about a 45-degree angle to your body.
- **Uneven Repetition**: Ensure both dumbbells are lifted symmetrically to avoid muscle imbalance.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=bf93PZWLeF8)
- [Form guide](https://www.muscleandstrength.com/exercises/incline-dumbbell-bench-press.html)'
 WHERE id = 22;

DELETE FROM exercise_muscle_groups WHERE exercise_id = 22;

INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary)
VALUES (22, 'Chest',      1),
       (22, 'Shoulders',  1),
       (22, 'Triceps',    0),
       (22, 'Upper Back', 0);

INSERT INTO exercises (id, name, category, exercise_type, description_markdown)
VALUES (24, 'Assisted Pull-Up', 'upper', 'assisted', '## Instructions
1. Set up the assistance: loop a resistance band over the pull-up bar and place one foot or knee in the loop, or use an assisted pull-up machine and select an assistance weight.
2. Grip the bar slightly wider than shoulder width with palms facing away.
3. Engage your lats and pull your chest toward the bar, keeping elbows tucked and shoulders down.
4. Lower yourself with control until your arms are fully extended.

## Common Mistakes
- **Swinging or kipping**: Use a controlled tempo throughout.
- **Half reps**: Lower all the way to a full hang to train the full range.
- **Shrugged shoulders**: Pull your shoulder blades down and back before each rep.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=eGo4IYlbE5g)
- [Form guide](https://www.verywellfit.com/how-to-do-the-assisted-pull-up-3498379)

## Tracking your progress
Check the **Assisted** box and enter the assistance amount as a positive number — the app stores it as negative weight. As you get stronger, reduce the assistance. Once you can do unassisted reps, leave the box unchecked. To progress further, add weight with a belt and continue with the box unchecked.');

INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary)
VALUES (24, 'Lats',       1),
       (24, 'Biceps',     1),
       (24, 'Upper Back', 0),
       (24, 'Forearms',   0);

COMMIT;
