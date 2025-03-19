INSERT INTO muscle_groups (name)
VALUES
-- Upper Body
('Chest'),
('Shoulders'),
('Triceps'),
('Biceps'),
('Upper Back'),
('Lats'),
('Traps'),
('Forearms'),
-- Core
('Abs'),
('Obliques'),
('Lower Back'),
-- Lower Body
('Quads'),
('Hamstrings'),
('Glutes'),
('Calves'),
('Hip Flexors'),
('Adductors') ON CONFLICT(name) DO
UPDATE SET
    name = excluded.name;

INSERT INTO exercises (id, name, category, description_markdown)
values  (1, 'Deadlift', 'full_body', '## Instructions
1. Stand with your feet about hip-width apart, placed under the barbell.
2. Bend at the hips and knees to grip the bar with your hands slightly wider than shoulder-width apart. Keep your chest up and back straight.
3. Engage your core and drive through your heels to lift the bar, extending your hips and knees simultaneously until you''re standing upright.
4. Breathe in before initiating the lift, exhale as you reach the top, and take care not to hyperextend your back at lockout.
5. Lower the bar by hinging at the hips and bending your knees, maintaining a straight back, until the weights touch the ground.

## Common Mistakes
- Rounding the back: Keep your back straight and chest up to prevent strain.
- Jerking the weight: Use a smooth, controlled motion to initiate the lift.
- Lifting with the arms: Focus on driving through the heels and using your legs and hips.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=op9kVnSso6Q)
- [Physiopedia](https://www.physio-pedia.com/Deadlift_Exercise)'),
        (2, 'Bench Press', 'upper', '## Instructions
1. Lie flat on a bench with feet firmly planted on the ground. Position your eyes directly under the bar.
2. Grip the barbell with hands slightly wider than shoulder-width apart, ensuring wrists are aligned and firm.
3. Unrack the bar and hold it directly above your chest with arms fully extended.
4. Lower the bar slowly to your chest by bending elbows, keeping them at a 45-degree angle to your torso.
5. Press the bar back up to starting position by extending your arms, exhaling as you push through.

## Common Mistakes
- **Arching the back excessively**: Maintain a natural arch by keeping the back firm and core engaged throughout.
- **Flared elbows**: Keep elbows at 45 degrees to prevent shoulder strain.
- **Bouncing the bar off the chest**: Focus on controlling the bar''s descent and pressing smoothly.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=hWbUlkb5Ms4)
- [Physiopedia](https://www.physio-pedia.com/Bench_Press)'),
        (3, 'Tricep Pushdown', 'upper', '## Instructions
1. Begin by adjusting the cable machine so the pulley is at the highest setting. Attach a short, straight bar or rope to the cable.
2. Stand facing the machine, feet shoulder-width apart. Grasp the bar or rope with an overhand grip, keeping your elbows close to your sides.
3. With a slight bend in your knees for stability, push down by extending your elbows until your arms are fully straightened, without locking your joints.
4. Slowly return to the starting position, controlling the weight throughout the motion. Remember to exhale during the pushdown and inhale while returning.

## Common Mistakes
- **Elbow Flare:** Ensure elbows remain close to the body to target the triceps effectively.
- **Swinging the Body:** Keep torso still to prevent using momentum instead of muscle strength.
- **Wrist Bending:** Maintain a neutral wrist position to reduce strain and focus on the triceps.

## Resources
- [Video tutorial (bar)](https://www.youtube.com/watch?v=6Fzep104f0s)
- [Video tutorial (rope)](https://www.youtube.com/watch?v=-xa-6cQaZKY)
- [Form guide](https://www.verywellfit.com/how-to-do-the-triceps-pushdown-3498613)'),
        (4, 'Dumbbell Biceps Curl', 'upper', '## Instructions
1. Stand up straight with a pair of dumbbells in your hands, arms hanging at your sides, palms facing forward.
2. Keep your elbows close to your body and your feet hip-width apart. Engage your core for balance.
3. Curl the weights by bending your elbows while keeping the upper arms stationary. Lift until your biceps are fully contracted, and the dumbbells are at shoulder height.
4. Pause at the top for a moment. Focus on breathing steadily without holding your breath.
5. Slowly lower the dumbbells back to the starting position, ensuring a controlled movement.

## Common Mistakes
- Swinging the dumbbells: Focus on keeping your elbows stationary and using only your biceps to lift the weights.
- Using momentum: Maintain a controlled pace during upward and downward movements.
- Gripping too tightly: A relaxed grip will avoid unnecessary tension in the forearms.
- Bending the wrists: Keep wrists straight to prevent strain.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=HnHuhf4hEWY)
- [Form guide](https://www.verywellfit.com/how-to-do-the-biceps-arm-curl-3498604)'),
        (5, 'Lateral Raise', 'upper', '## Instructions
1. **Stand tall and grasp** a pair of dumbbells with a neutral grip, letting your arms hang naturally by your sides.
2. **Position your feet** shoulder-width apart, maintaining a slight bend in your elbows.
3. **Raise your arms laterally** until they are parallel to the floor, keeping elbows slightly bent; do not swing your arms. Ensure shoulders remain relaxed.
4. **Lower** the dumbbells slowly back to the starting position, exhaling as you do so.

## Common Mistakes
- **Using momentum:** Avoid swaying or using your back to lift weights. Engage your core for stability.
- **Lifting too high:** Lifting above shoulder level can cause shoulder joint strain. Stop when they are parallel to the floor.
- **Tense neck:** Relax your neck to prevent tension and potential strain.
- **Incorrect posture:** Ensure your back remains neutral, not arched, to avoid injury.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=OuG1smZTsQQ)
- [Form guide](https://www.verywellfit.com/side-lateral-raise-4588211)
'),
        (6, 'Dumbbell Shoulder Press', 'upper', '## Instructions
1. **Position yourself upright**: Sit on a bench with back support. Hold a dumbbell in each hand, resting them just above the shoulders with your palms facing forward.
2. **Feet firm on the floor**: Keep your feet flat and planted for stability, ensuring your core is engaged.
3. **Press upward**: Push the weights above your head until your arms are straight and elbows are slightly bent but not locked.
4. **Controlled movement**: Lower the dumbbells slowly back to the starting position.
5. **Breathing**: Exhale as you press up, inhale as you lower the weights down.

## Common Mistakes
- **Overarching the back**: Maintain a neutral spine; avoid leaning back excessively.
- **Elbows flaring out**: Keep elbows in line with your torso, avoiding excessive outward motion.
- **Using momentum**: Focus on a controlled lift and descent to engage the muscles properly.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=Did01dFR3Lk)
- [Form guide](https://ericrobertsfitness.com/how-to-do-dumbbell-shoulder-press-the-correct-guide/)'),
        (7, 'Dumbbell Bench Press', 'upper', '## Instructions
1. **Starting Position**: Lie flat on a bench, feet planted firmly on the ground. Hold a dumbbell in each hand with an overhand grip and extend your arms above your chest with the elbows slightly bent.
2. **Body Alignment**: Keep your back flat against the bench and engage your core. Ensure your shoulder blades are retracted for stability.
3. **Perform the Lift**: Lower the dumbbells slowly to the sides of your chest, keeping your elbows at a 45-degree angle to your torso. Inhale as you lower the weights.
4. **Press Up**: Press the dumbbells back up while exhaling, straightening your arms without locking elbows.

## Common Mistakes
- **Flared Elbows**: Avoid excessively flaring your elbows outward, as this can strain shoulders. Keep them at a comfortable angle.
- **Arching Back**: Ensure your back stays flat against the bench to prevent lower back issues.
- **Rapid Movements**: Don''t rush; a slow, controlled motion engages muscles effectively and reduces injury risk.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=YQ2s_Y7g5Qk)
- [Form guide](https://www.muscleandstrength.com/exercises/dumbbell-bench-press.html)'),
        (8, 'Cable Fly', 'upper', '## Instructions
1. **Set-Up**: Attach handles to the upper cables on both sides of a cable machine station. Adjust the weight stack to suit your strength level.
2. **Positioning**: Stand in the center of the station with feet shoulder-width apart. Grip each handle and step forward, keeping arms outstretched and elbows slightly bent.
3. **Execution**: Exhale and bring the handles together in front of you in a wide arc, keeping a slight bend in your elbows.
4. **Returning Motion**: Inhale and slowly return to the starting position by extending your arms back to your sides.

## Common Mistakes
- **Rounded Shoulders**: Avoid rounding your shoulders; maintain a proud chest and retracted scapulae.
- **Elbows Locked**: Do not lock elbows; keep a slight bend to maintain tension and avoid injury.
- **Swinging Movement**: Do not use momentum to move the weights; focus on controlled muscle-driven movements.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=4mfLHnFL0Uw)
- [Form guide](https://www.muscleandstrength.com/exercises/cable-crossovers-(mid-chest).html)
'),
        (9, 'Pulldown', 'upper', '## Instructions
1. **Adjust the seat to ensure your body is stable**: Sit down and secure your thighs under the padded restraint. Make sure your feet are flat on the floor and your hips are aligned with your spine.
2. **Grip the bar with an overhand grip**: Reach up and grasp the pulldown bar with your palms facing away, hands slightly wider than shoulder-width apart.
3. **Engage your back and pull the bar down**: With a slight lean back, focus on using your back muscles, especially your lats, to pull the bar down to your upper chest, ensuring your elbows remain alongside your body.
4. **Exhale on the pull-down and keep control**: Exhale as you pull down, pause briefly at the bottom of the movement, then slowly return the bar to starting position, controlling the ascent.

## Common Mistakes
- **Using too much momentum**: Avoid swinging your torso; instead, use a controlled motion to isolate the lats.
- **Incorrect grip width**: Ensure your hands are positioned slightly wider than shoulder-width to target the correct muscles effectively.
- **Not bringing the bar to the chest**: Failing to pull the bar to the chest may reduce muscle engagement. Ensure the bar reaches upper chest level.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=AOpi-p0cJkc)
- [Form guide](https://www.verywellfit.com/how-to-do-the-lat-pulldown-3498309)
'),
        (10, 'Pulldown, Reverse Grip', 'upper', '## Instructions
1. Start by adjusting the thigh pad on the lat pulldown machine to secure your legs. Stand up and grasp the bar with an underhand grip, hands shoulder-width apart.
2. Sit down, ensuring your thighs are snug under the pad and your back is straight. Arch your back slightly and puff your chest out.
3. Pull the bar straight down towards your chest, keeping your elbows close to your sides. Focus on squeezing your shoulder blades together.
4. Breathe out as you pull the bar down, hold for a moment, then slowly return to the start position.

## Common Mistakes
- **Using momentum by swinging the torso during the pull.** Keep your body stable to target your muscles effectively.
- **Pulling the bar down too far.** Stop at chest level to avoid unnecessary strain on your shoulders and back.
- **Flared elbows.** Keep elbows close to your body to better engage the lats.
- **Gripping the bar too wide.** Shoulder-width grip optimizes the range of motion.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=VprlTxpB1rk)
- [Form guide](https://www.puregym.com/exercises/back/lat-exercises/reverse-grip-lat-pulldown/)
'),
        (11, 'Seated Cable Row', 'upper', '## Instructions
1. **Sit down at the seated cable row machine**: Ensure your feet are planted firmly on the foot plates, with knees slightly bent. Keep your back straight.
2. **Grab the handle**: Hold the V-bar or straight bar with an overhand or neutral grip, making sure your palms face each other.
3. **Engage your core**: Pull your shoulders back and down, maintaining a strong and stable core.
4. **Execute the movement**: Exhale as you pull the handle towards your torso, keeping your elbows close to your body. Avoid using momentum. Focus on squeezing your shoulder blades together during this phase.
5. **Return with control**: Inhale as you slowly extend your arms forward, keeping slight tension in your muscles throughout the movement.

## Common Mistakes
- **Rounding the back**: Ensure a neutral spine by keeping your chest up and shoulders back. Poor posture can lead to lower back strain.
- **Using momentum**: Focus on controlled movements. Using momentum reduces muscle engagement and increases the risk of injury.
- **Elbows flaring out**: Keep your elbows close to your sides to maximize upper back engagement.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=UCXxvVItLoM)
- [Form guide](https://www.verywellfit.com/how-to-do-the-cable-row-3498605)'),
        (12, 'One-Arm Dumbell Row', 'upper', '## Instructions
1. **Start Position:** Stand with feet shoulder-width apart. Place your left knee and hand on a bench, your back parallel to the floor. Hold the dumbbell in your right hand, arm extended down.
2. **Grip & Alignment:** Keep your elbow slightly bent, align your wrists with your forearms, and square your shoulders.
3. **Row Motion:** Pull the dumbbell towards your hip, keeping the elbow close. Squeeze your shoulder blades at the top. Pause and lower it back down slowly.
4. **Breathing:** Breathe out as you pull, and go slow. Inhale when returning to the start position.
5. **Repetitions:** Aim for 8-12 reps, switch sides, and repeat to target both sides evenly.

## Common Mistakes
- **Rounded Back:** Engaging your core, maintain a flat back throughout to reduce strain.
- **Swung Arm:** Focus on controlled motion, avoid jerking or swinging the dumbbell.
- **Elbow Flare:** Keep the elbow close, avoiding unnecessary stress on the shoulder joints.
- **Rushed Reps:** Ensure a steady tempo to maximize muscle engagement and form accuracy.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=DMo3HJoawrU)
- [Form guide](https://www.verywellfit.com/one-arm-dumbbell-row-3120040)'),
        (13, 'Abdominal Machine Crunch', 'upper', '## Instructions
1. **Adjust the Machine:** Sit comfortably on the abdominal machine and ensure that your feet are flat on the ground or on the footholds. Adjust the seat height so that the pads rest on your upper chest or shoulders.
2. **Grip the Handles:** Grasp the handles firmly with your hands or position them across your chest if the machine design allows.
3. **Engage Your Core:** Lean back slightly and engage your core muscles to avoid using your legs to assist in the movement.
4. **Crunch Forward:** Exhale as you use your abs to pull the top pad downwards towards your lower body. Hold at the peak contraction for a moment to maximize the engagement.
5. **Return Slowly:** Inhale and slowly return to the starting position, maintaining control of the weight.

## Common Mistakes
- **Using Momentum:** Avoid using jerky movements or swinging; focus on a slow and controlled motion to ensure maximum muscle engagement.
- **Inadequate Adjustment:** Ensure the seat and pads are correctly adjusted to avoid discomfort or ineffective muscle engagement.
- **Neglecting Form:** Maintain a tight core throughout and avoid overextending your back on the return to starting position.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=fuPFq2EYswE)
- [Form guide](https://kinxlearning.com/pages/abdominal-machine-crunch?srsltid=AfmBOop6rS1Lir1Vh5C8c8ZrDsmuiU7TZpSB3thYX-uMwML4bcEc1_WC)'),
        (14, 'Leg Press', 'lower', '## Instructions
1. **Position your feet**: Sit on the leg press machine and place your feet shoulder-width apart on the platform. Keep your back flat against the seat with a neutral spine.
2. **Set up the machine**: Ensure that your knees form a 90-degree angle to prevent undue knee strain. Grip the side handles for stability.
3. **Execute the movement**: Press the platform away by extending your legs without locking out your knees. Pause briefly at the top.
4. **Controlled return**: Lower the weight back towards you just until your knees reach a 90-degree angle, maintaining tension in your legs throughout.
5. **Breathing and pace**: Breathe out as you press the weight up and inhale while returning it. Maintain a steady, controlled pace throughout each rep.

## Common Mistakes
- **Arching the back**: Keep your back flat against the seat to avoid strain. Ensure a tight core.
- **Knees caving in**: Prevent this by pushing through the heels and aligning knees over toes.
- **Locking out knees**: Avoid full extension to keep tension and reduce joint stress.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=VFk3RzndUEc)
- [Form guide](https://www.verywellfit.com/how-to-do-the-leg-press-3498610)'),
        (15, 'Leg Extension', 'lower', '## Instructions
1. **Starting Position:** Sit on the leg extension machine, aligning your knee joints with the machine''s pivot point. Adjust the backrest so that your back is fully supported.
2. **Foot Positioning:** Place your feet under the padded bar, which should rest on your shins just above the ankle.
3. **Movement Execution:** Extend your legs by straightening your knees until they are fully extended. Keep your back pressed against the machine pad and your movements controlled.
4. **Hold and Reverse:** Hold the extended position for a second, then slowly return to the starting position without letting the weights touch the stack.
5. **Breathing/Tempo:** Exhale as you extend your legs and inhale as you return to the start. Aim for 8-12 repetitions over 3 sets.

## Common Mistakes
- **Knee Overextension:** Do not lock out your knees at the top. Maintain a slight bend to avoid joint stress.
- **Jerky Movements:** Always move the weights with control. Avoid using momentum by keeping a steady pace.
- **Incorrect Seat Adjustment:** Ensure your knees are properly aligned with the machine’s pivot to reduce injury risk.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=4ZDm5EbiFI8)
- [Form guide](https://www.verywellfit.com/leg-extensions-benefit-or-risk-3498573)'),
        (16, 'Leg Curl', 'lower', '## Instructions
1. **Setup:** Adjust the machine so that the padded lever is comfortably resting on the back of your ankles. Sit or lie flat on your stomach with your legs fully extended.
2. **Positioning:** Ensure your knees are aligned with the pivot point of the machine. Keep your torso flat against the bench, and hold onto the handles or sides of the bench for stability.
3. **Movement:** Flex your knees and slowly curl your legs up towards your glutes while exhaling. Keep your toes pointed at the ceiling and control the movement.
4. **Breathing:** Inhale as you slowly return to the starting position, ensuring a controlled motion throughout.

## Common Mistakes
- **Partial Range of Motion:** Avoid doing half reps; ensure the range covers full knee flexion for effectiveness.
- **Fast Tempo**: Prevent jerky movements by maintaining a slow and steady tempo for muscle engagement.
- **Off-Alignment**: Check that your knees are not misaligned with the machine pivot which may cause strain.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=t9sTSr-JYSs)
- [Form guide](https://www.verywellfit.com/how-to-properly-execute-the-leg-curl-exercise-3498304)'),
        (17, 'Calf Raise', 'lower', '## Instructions
1. Stand upright with your feet hip-width apart. Keep your shoulders back and your core engaged to maintain balance.
2. Place the balls of your feet on the edge of a step or platform, with your heels extending off the edge slightly. Hold onto a railing or wall for additional support.
3. Raise your heels off the ground as high as possible by contracting your calf muscles. Hold the top position for a brief moment.
4. Slowly lower your heels back to the starting position, allowing your calves to stretch slightly.

## Common Mistakes
- **Bouncing**: Performing the movement too quickly, causing a bounce. Focus on a slow and controlled motion.
- **Incomplete Range of Motion**: Not fully elevating the heels or lowering them enough. Ensure full extension and contraction of the calves.
- **Leaning Forward**: Using too much momentum by leaning the upper body forward. Keep an upright posture throughout.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=eMTy3qylqnE)
- [Form guide](https://steelsupplements.com/blogs/steel-blog/how-to-do-smith-machine-calf-raises-form-and-benefits)
- [Calf Workout Variations](https://blog.myarsenalstrength.com/calf-raise-variations)'),
        (18, 'Back Extension', 'lower', '## Instructions
1. **Position Yourself:** Begin by lying face down on a hyperextension bench. Adjust the padded supports so that your hips are just off the edge and your feet are secured.
2. **Neutral Spine:** Cross your arms over your chest or place your hands behind your head. Ensure your spine is in a neutral position.
3. **Begin Extension:** Slowly lift your upper body using the muscles of your lower back until your body forms a straight line.
4. **Breath and Hold:** Inhale on the ascent and hold for a moment at the top position without arching your back excessively.
5. **Controlled Descent:** Exhale as you return to the starting position in a controlled manner.

## Common Mistakes
- **Excessive Arching:** Avoid arching your back too much at the top. Focus on a neutral spine.
- **Fast Movements:** Perform the exercise in a slow, controlled manner to prevent injury.
- **Incorrect Machine Setup:** Ensure the bench is set up so that it allows full range of motion without strain.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=ENXyYltB7CM)
- [Form guide](https://www.healthline.com/health/back-extension-exercise#with-weight)
')
ON CONFLICT(id) DO UPDATE SET
    name = excluded.name,
                       category = excluded.category,
                       description_markdown = excluded.description_markdown;

INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary)
VALUES
(1, 'Forearms', 0),
(1, 'Glutes', 1),
(1, 'Hamstrings', 1),
(1, 'Lats', 0),
(1, 'Lower Back', 1),
(1, 'Quads', 0),
(1, 'Traps', 0),
(1, 'Upper Back', 0),
(2, 'Abs', 0),
(2, 'Chest', 1),
(2, 'Forearms', 0),
(2, 'Shoulders', 1),
(2, 'Triceps', 1),
(2, 'Upper Back', 0),
(3, 'Shoulders', 0),
(3, 'Triceps', 1),
(4, 'Biceps', 1),
(4, 'Forearms', 0),
(5, 'Shoulders', 1),
(5, 'Traps', 0),
(5, 'Upper Back', 0),
(6, 'Shoulders', 1),
(6, 'Triceps', 0),
(6, 'Upper Back', 0),
(7, 'Chest', 1),
(7, 'Shoulders', 0),
(7, 'Triceps', 0),
(8, 'Chest', 1),
(8, 'Shoulders', 0),
(8, 'Triceps', 0),
(9, 'Biceps', 0),
(9, 'Lats', 1),
(9, 'Shoulders', 0),
(9, 'Upper Back', 1),
(10, 'Biceps', 1),
(10, 'Forearms', 0),
(10, 'Lats', 1),
(10, 'Upper Back', 0),
(11, 'Biceps', 0),
(11, 'Lats', 1),
(11, 'Lower Back', 0),
(11, 'Upper Back', 1),
(12, 'Biceps', 0),
(12, 'Forearms', 0),
(12, 'Lats', 1),
(12, 'Upper Back', 1),
(13, 'Abs', 1),
(13, 'Obliques', 0),
(14, 'Calves', 0),
(14, 'Glutes', 1),
(14, 'Hamstrings', 0),
(14, 'Quads', 1),
(15, 'Hip Flexors', 0),
(15, 'Quads', 1),
(16, 'Calves', 0),
(16, 'Hamstrings', 1),
(17, 'Calves', 1),
(17, 'Quads', 0),
(18, 'Glutes', 0),
(18, 'Hamstrings', 0),
(18, 'Lower Back', 1)
ON CONFLICT(exercise_id, muscle_group_name) DO UPDATE SET
    is_primary = excluded.is_primary;
