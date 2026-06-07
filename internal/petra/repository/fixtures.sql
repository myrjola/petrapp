-- Idempotent renames: applied before the main INSERT so prod rows match the new
-- names before the INSERT's ON CONFLICT(name) clause has to resolve them. Pure
-- (id,name) renames can't go through ON CONFLICT directly because the conflict
-- would be on the PK, not on name.
UPDATE exercises SET name = 'One-Arm Dumbbell Row' WHERE name = 'One-Arm Dumbell Row';
UPDATE exercises SET name = 'Push-Up'              WHERE name = 'Push-up';

INSERT INTO muscle_groups (name)
VALUES
-- Upper Body
('Chest'),
('Shoulders'),
('Side Delts'),
('Rear Delts'),
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
('Adductors') ON CONFLICT(name) DO
UPDATE SET name = excluded.name;

INSERT INTO exercises (id, name, category, exercise_type, description_markdown, rep_min, rep_max)
VALUES (1, 'Deadlift', 'full_body', 'weighted', '## Instructions
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
- [Video tutorial](https://www.youtube.com/watch?v=op9kVnSso6Q)', 3, 6),
       (2, 'Bench Press', 'upper', 'weighted', '## Instructions
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
- [Video tutorial](https://www.youtube.com/watch?v=hWbUlkb5Ms4)', 5, 10),
       (3, 'Tricep Pushdown', 'upper', 'weighted', '## Instructions
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
- [Video tutorial (rope)](https://www.youtube.com/watch?v=-xa-6cQaZKY)', 8, 12),
       (4, 'Dumbbell Biceps Curl', 'upper', 'weighted', '## Instructions
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
- [Video tutorial](https://www.youtube.com/watch?v=HnHuhf4hEWY)', 8, 12),
       (5, 'Lateral Raise', 'upper', 'weighted', '## Instructions
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
', 10, 20),
       (6, 'Dumbbell Shoulder Press', 'upper', 'weighted', '## Instructions
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
- [Video tutorial](https://www.youtube.com/watch?v=Did01dFR3Lk)', 5, 10),
       (7, 'Dumbbell Bench Press', 'upper', 'weighted', '## Instructions
1. **Starting Position**: Lie flat on a bench, feet planted firmly on the ground. Hold a dumbbell in each hand with an overhand grip and extend your arms above your chest with the elbows slightly bent.
2. **Body Alignment**: Keep your back flat against the bench and engage your core. Ensure your shoulder blades are retracted for stability.
3. **Perform the Lift**: Lower the dumbbells slowly to the sides of your chest, keeping your elbows at a 45-degree angle to your torso. Inhale as you lower the weights.
4. **Press Up**: Press the dumbbells back up while exhaling, straightening your arms without locking elbows.

## Common Mistakes
- **Flared Elbows**: Avoid excessively flaring your elbows outward, as this can strain shoulders. Keep them at a comfortable angle.
- **Arching Back**: Ensure your back stays flat against the bench to prevent lower back issues.
- **Rapid Movements**: Don''t rush; a slow, controlled motion engages muscles effectively and reduces injury risk.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=YQ2s_Y7g5Qk)', 5, 10),
       (8, 'Cable Fly', 'upper', 'weighted', '## Instructions
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
', 8, 12),
       (9, 'Pulldown', 'upper', 'weighted', '## Instructions
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
', 5, 10),
       (10, 'Pulldown, Reverse Grip', 'upper', 'weighted', '## Instructions
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
', 5, 10),
       (11, 'Seated Cable Row', 'upper', 'weighted', '## Instructions
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
- [Video tutorial](https://www.youtube.com/watch?v=UCXxvVItLoM)', 5, 10),
       (12, 'One-Arm Dumbbell Row', 'upper', 'weighted', '## Instructions
1. **Start Position:** Stand with feet shoulder-width apart. Place your left knee and hand on a bench, your back parallel to the floor. Hold the dumbbell in your right hand, arm extended down.
2. **Grip & Alignment:** Keep your elbow slightly bent, align your wrists with your forearms, and square your shoulders.
3. **Row Motion:** Pull the dumbbell towards your hip, keeping the elbow close. Squeeze your shoulder blades at the top. Pause and lower it back down slowly.
4. **Breathing:** Breathe out as you pull, and go slow. Inhale when returning to the start position.

## Common Mistakes
- **Rounded Back:** Engaging your core, maintain a flat back throughout to reduce strain.
- **Swung Arm:** Focus on controlled motion, avoid jerking or swinging the dumbbell.
- **Elbow Flare:** Keep the elbow close, avoiding unnecessary stress on the shoulder joints.
- **Rushed Reps:** Ensure a steady tempo to maximize muscle engagement and form accuracy.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=DMo3HJoawrU)', 5, 10),
       (13, 'Abdominal Machine Crunch', 'upper', 'weighted', '## Instructions
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
- [Form guide](https://kinxlearning.com/pages/abdominal-machine-crunch?srsltid=AfmBOop6rS1Lir1Vh5C8c8ZrDsmuiU7TZpSB3thYX-uMwML4bcEc1_WC)', 8, 15),
       (14, 'Leg Press', 'lower', 'weighted', '## Instructions
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
- [Video tutorial](https://www.youtube.com/watch?v=VFk3RzndUEc)', 5, 10),
       (15, 'Leg Extension', 'lower', 'weighted', '## Instructions
1. **Starting Position:** Sit on the leg extension machine, aligning your knee joints with the machine''s pivot point. Adjust the backrest so that your back is fully supported.
2. **Foot Positioning:** Place your feet under the padded bar, which should rest on your shins just above the ankle.
3. **Movement Execution:** Extend your legs by straightening your knees until they are fully extended. Keep your back pressed against the machine pad and your movements controlled.
4. **Hold and Reverse:** Hold the extended position for a second, then slowly return to the starting position without letting the weights touch the stack.

## Common Mistakes
- **Knee Overextension:** Do not lock out your knees at the top. Maintain a slight bend to avoid joint stress.
- **Jerky Movements:** Always move the weights with control. Avoid using momentum by keeping a steady pace.
- **Incorrect Seat Adjustment:** Ensure your knees are properly aligned with the machine’s pivot to reduce injury risk.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=4ZDm5EbiFI8)', 8, 12),
       (16, 'Leg Curl', 'lower', 'weighted', '## Instructions
1. **Setup:** Adjust the machine so that the padded lever is comfortably resting on the back of your ankles. Sit or lie flat on your stomach with your legs fully extended.
2. **Positioning:** Ensure your knees are aligned with the pivot point of the machine. Keep your torso flat against the bench, and hold onto the handles or sides of the bench for stability.
3. **Movement:** Flex your knees and slowly curl your legs up towards your glutes while exhaling. Keep your toes pointed at the ceiling and control the movement.
4. **Breathing:** Inhale as you slowly return to the starting position, ensuring a controlled motion throughout.

## Common Mistakes
- **Partial Range of Motion:** Avoid doing half reps; ensure the range covers full knee flexion for effectiveness.
- **Fast Tempo**: Prevent jerky movements by maintaining a slow and steady tempo for muscle engagement.
- **Off-Alignment**: Check that your knees are not misaligned with the machine pivot which may cause strain.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=t9sTSr-JYSs)', 8, 12),
       (17, 'Calf Raise', 'lower', 'weighted', '## Instructions
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
- [Form guide](https://steelsupplements.com/blogs/steel-blog/how-to-do-smith-machine-calf-raises-form-and-benefits)', 10, 20),
       (18, 'Back Extension', 'lower', 'weighted', '## Instructions
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
', 8, 20),
       (19, 'Push-Up', 'upper', 'bodyweight', '## Instructions
1. Start in a plank position with hands placed slightly wider than shoulder-width apart, arms straight.
2. Keep your body in a straight line from head to heels, engaging your core throughout the movement.
3. Lower your body by bending your elbows until your chest nearly touches the floor.
4. Push back up to the starting position by straightening your arms, exhaling as you push up.

## Common Mistakes
- Sagging hips: Keep your core engaged to maintain a straight line from head to heels.
- Flared elbows: Keep elbows at a 45-degree angle to reduce shoulder strain.
- Partial range of motion: Lower your chest close to the floor for full muscle engagement.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=IODxDxX7oi4)
', 5, 10),
       (20, 'Ab Wheel Rollout', 'upper', 'bodyweight', '## Instructions
1. Start on your knees with the ab wheel in your hands, positioned under your shoulders.
2. Keep your core tight and slowly roll the wheel forward, extending your body into a straight line.
3. Roll out as far as you can while maintaining control and keeping your back straight.
4. Use your core muscles to pull yourself back to the starting position.

## Common Mistakes
- Arching the back: Keep your core engaged to prevent lower back strain.
- Rolling too far: Only go as far as you can maintain proper form and control.
- Using momentum: Focus on controlled movements using your core muscles.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=UNjRzDhWgLU)
', 8, 15),
       (21, 'Plank', 'upper', 'bodyweight', '## Instructions
1. Start in a push-up position but rest on your forearms instead of your hands.
2. Keep your body in a straight line from head to heels, engaging your core and glutes.
3. Hold this position for the specified time, breathing normally throughout.
4. Keep your neck neutral by looking down at the floor.

## Common Mistakes
- Sagging hips: Engage your core and glutes to maintain a straight line.
- Raised hips: Avoid creating a peak with your hips; keep body straight.
- Holding breath: Remember to breathe normally during the hold.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=ASdvN_XEl_c)
', 8, 15),
       (22, 'Incline Dumbbell Bench Press', 'upper', 'weighted', '## Instructions
1. **Bench Setup**: Adjust a bench to an incline of 30-45 degrees. Lie back on the bench while holding a dumbbell in each hand resting just above your chest.
2. **Foot Positioning**: Firmly plant your feet on the floor to provide stability through your legs and lower back. Your shoulder blades should be pulled back for support.
3. **Pressing Motion**: As you exhale, press the dumbbells upward until your arms are fully extended but not locked out.
4. **Return Movement**: Inhale as you slowly lower the dumbbells back to the starting position, keeping your elbows slightly below the shoulders.

## Common Mistakes
- **Arching Back**: Avoid excessive arching of the back by engaging your core and keeping your back flat against the bench.
- **Flared Elbows**: Tucking your elbows too far out can strain the shoulders. Keep them at about a 45-degree angle to your body.
- **Uneven Repetition**: Ensure both dumbbells are lifted symmetrically to avoid muscle imbalance.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=bf93PZWLeF8)', 5, 10),
       (23, 'Romanian Deadlift', 'lower', 'weighted', '## Instructions
1. **Starting Position**: Stand with your feet hip-width apart, holding a pair of dumbbells in front of your thighs or use a barbell with an overhand grip.
2. **Knee Alignment**: Slightly bend your knees, keeping your back straight. Your shoulders should be back and down, and your core engaged to maintain a neutral spine.
3. **Lowering the Weight**: Hinge at the hips to lower the dumbbells/barbell, sliding it along the front of your legs. Keep the barbell close to your shins.
4. **Hip Hinge Focus**: Push your hips back as you lower the weight, ensuring your back remains straight and your chest is open. Feel the stretch in your hamstrings.
5. **Return to Position**: Exhale and drive your hips forward to lift the weight back to the starting position, contracting your glutes and hamstrings.

## Common Mistakes
- **Rounding the Back**: Avoid curving your spine by engaging your core throughout the movement.
- **Excessive Knee Bend**: Keep your knees slightly bent but do not squat as you lower.
- **Shallow Movement**: Ensure you''re hinging at your hips and not just bending your waist.
- **Overextending the Back at the Top**: Finish the movement with a straight back, not leaning backwards.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=OjYtl68wxy4)', 8, 20),
       (24, 'Assisted Pull-Up', 'upper', 'assisted', '## Instructions
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

## Tracking your progress
Check the **Assisted** box and enter the assistance amount as a positive number — the app stores it as negative weight. As you get stronger, reduce the assistance. Once you can do unassisted reps, leave the box unchecked. To progress further, add weight with a belt and continue with the box unchecked.', 5, 10),
       (25, 'Hip Abductor', 'lower', 'weighted', '## Instructions
1. **Initial Position:** Sit on the hip abductor machine with your back firmly against the pad and feet placed on the footrests.
2. **Adjust Settings:** Adjust the lever to a weight you''re comfortable with and ensure the knee pads are positioned securely against your outer thighs.
3. **Movement:** Slowly push your legs apart against the resistance as far as comfortably possible, feeling the tension along your outer thighs.
4. **Return:** Gradually bring your legs back to the starting position without letting the weights slam.

## Common Mistakes
- **Poor Posture:** Avoid slouching. Keep your back straight and engage your core to maintain stability.
- **Fast Movements:** Don''t rush. Move slowly to maintain tension and control.
- **Excessive Weight:** Start with a lighter weight to ensure proper form and prevent strain.
- **Leaning Forward:** Sit upright or slightly back in the chair to effectively engage the glute muscles.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=mP-FfS2HC9Q)', 8, 12),
       (26, 'Hip Adductor', 'lower', 'weighted', '## Instructions
1. **Sit down at the hip adduction machine**: Ensure your back is firmly against the pad and your spine remains neutral. Position your feet on the footrests.
2. **Adjust the pads**: Set the thigh pads against your inner thighs, just above the knees. Grip the side handles for stability.
3. **Select appropriate weight**: Choose a weight that allows controlled movement without requiring momentum.
4. **Execute the movement**: Exhale as you pull the pads together by squeezing your inner thighs, bringing your legs inward in a controlled motion.
5. **Return with control**: Inhale as you slowly return to the starting position, maintaining tension throughout without letting the weights slam.

## Common Mistakes
- **Using momentum**: Avoid swinging or jerking motions. Focus on controlled, isolated movements.
- **Forward lean**: Don''t lean forward during the exercise; keep your back against the pad.
- **Too much weight**: Using excessive weight compromises form and increases injury risk.
- **Poor foot placement**: Keep feet firmly planted on the footrests throughout the movement.
- **Neglecting the stretch**: Return to the starting position without fully extending the legs; maintain some tension in the muscles.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=b-cxonq03vQ)
- [Extended guide](https://www.strengthlog.com/hip-adduction-machine/)
', 8, 12),
       (27, 'Rotary Torso', 'upper', 'weighted', '## Instructions
1. **Adjust the machine**: Set the lever on the side of the seat to the neutral position. Start with a light weight to learn proper form.
2. **Sit down**: Scoot your buttocks all the way back and ensure your lower back touches the backrest. Keep your spine neutral.
3. **Stabilize your lower body**: Tuck the leg pad between your knees to stabilize your body and prevent your hips from rotating.
4. **Grip and position**: Wrap your arms around the two large pads and grip the handles. Keep your feet planted firmly.
5. **Execute the rotation**: Evenly rotate your upper body in one direction, keeping your core engaged and your lower body stable. Move slowly and with control throughout the entire range of motion.
6. **Return to center**: Slowly return to the starting position, maintaining control without momentum.
7. **Repeat other side**: Complete all reps on one side, then rotate to the opposite direction.

## Common Mistakes
- **Using momentum**: Avoid jerking or swinging motions. Smooth, controlled movements maximize core engagement.
- **Moving your hips**: Keep your lower body stable and pressed into the seat. Hip movement reduces core isolation.
- **Excessive rotation**: Only rotate within a safe, pain-free range. Forcing rotation can stress the spine.
- **Too much weight**: If you need momentum to start the movement, reduce the weight.
- **Shallow range of motion**: Rotate fully but controlled to maximize oblique engagement.
- **Feet leaving the floor**: Keep your feet planted to maintain stability throughout.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=V6K9bt4zbjc)
- [Form guide](https://training.fit/exercise/rotary-torso-machine/)
- [Extended guide](https://www.curves.com/blog/move/january-featured-machine-rotary-torso)
', 8, 15),
       (28, 'Seated Calf Raise', 'lower', 'weighted', '## Instructions
1. **Sit on the machine with your knees bent and feet firmly on the footplate.** Adjust the pad to rest comfortably on your thighs.
2. **Keep your feet hip-width apart and position your toes at the edge of the footplate.** Ensure your back is straight and hands are gripping the handles for stability.
3. **Push through the balls of your feet to raise the weight as high as possible.** Squeeze your calves at the top of the movement.
4. **Slowly lower the weight back down to the starting position, ensuring a controlled descent.**

## Common Mistakes
- **Lifting too quickly:** Maintain control to avoid momentum; focus on slow and steady movements.
- **Not using full range:** Extend fully at the top for optimum muscle activation.
- **Poor foot placement:** Keep feet planted and centered on the footplate for balance.
- **Using momentum:** Avoid bouncing or using your legs to propel the weight.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=ORY-ke6vcgk)', 10, 20),
       (29, 'Squat', 'lower', 'weighted', '## Instructions
1. **Setup**: Stand with feet shoulder-width apart, toes slightly pointing out. Engage your core and set your eyes ahead.
2. **Positioning**: Keep your chest up and retract your scapula while maintaining a neutral spine.
3. **Movement**: Lower your body by bending your knees and hips as if sitting back into a chair, keeping your weight in the heels.
4. **Breathing**: Inhale as you lower down and exhale as you push through the heels to stand back up.

## Common Mistakes
- **Knees Caving In**: Keep knees aligned with toes to maintain proper form.
- **Leaning Forward**: Maintain an upright torso and let your hips come down naturally.
- **Heels Lifting Off**: Keep weight distributed evenly across your feet.
- **Feet Too Narrow or Wide**: Adjust to shoulder width for optimal stability and form.
- **Rushing the Movement**: Move with control and maintain steady tempo.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=f-KL4VNN96E)', 3, 6),
       (30, 'Pec Fly', 'upper', 'weighted', '## Instructions
1. Sit at a pec fly machine or lie on a bench with dumbbells, keeping your feet flat, chest up, and shoulders pulled down and back.
2. Start with your arms open wide at chest level, elbows slightly bent and locked in that soft bend throughout the set.
3. Bring your hands together in a wide hugging motion until they meet in front of your chest, focusing on squeezing your chest instead of shrugging your shoulders.
4. Inhale as you return slowly to the start, lowering only until you feel a light chest stretch without shoulder pain. Move with control and avoid bouncing.
5. Perform smooth, controlled repetitions for your target reps, using a weight you can manage without losing form.

## Common Mistakes
- Bending and straightening the elbows too much: keep a small fixed bend so the chest does the work.
- Lowering too far: going past a comfortable stretch can stress the shoulder joint; stop if you feel pain.
- Shrugging the shoulders up: keep shoulders down and back to protect the neck and improve chest activation.
- Using too much weight: reduce load if you cannot control the motion evenly.

## Resources
- [How To: Pec Fly (Machine)](https://www.youtube.com/watch?v=Z57CtFmRMxA)', 8, 12),
       (31, 'Smith Machine Squat', 'lower', 'weighted', '## Instructions
1. Set the bar on the Smith machine to upper chest height and stand under it with the bar resting across your upper traps, not your neck. Place your feet slightly forward, about shoulder-width apart, with toes slightly turned out.
2. Unrack the bar and brace your core. Keep your chest up, back neutral, and knees tracking in line with your toes throughout the lift.
3. Lower by bending your hips and knees until your thighs are at least parallel to the floor, or as low as you can control without your heels lifting or lower back rounding.
4. Exhale and drive through your midfoot and heels to stand back up. Move at a steady tempo and avoid bouncing at the bottom.
5. Repeat for controlled reps, re-racking the bar securely at the end of the set.

## Common Mistakes
- Letting the knees cave inward: push them out gently so they stay aligned with your toes.
- Placing the feet too close under the bar: move them slightly forward to match the machine path and keep balance.
- Rounding the lower back: brace your core and only squat as deep as you can maintain a neutral spine.
- Rising onto the toes: keep pressure through the midfoot and heels for better stability.

## Resources
- [How To: Smith Machine Squat](https://www.youtube.com/watch?v=JQW3x5W4n9M)', 3, 6),
       (32, 'Overhead Press', 'upper', 'weighted', '## Instructions
1. Stand with feet about shoulder-width apart and hold a barbell at upper chest height with hands just outside shoulder width. Keep wrists stacked over elbows and brace your core.
2. Squeeze your glutes, keep your ribs down, and start with the bar close to your body. Your forearms should be vertical and your head neutral.
3. Press the bar straight overhead until your arms are fully extended. As the bar passes your forehead, move your head slightly back through so the bar finishes over the middle of your foot.
4. Exhale as you press up, then inhale and lower the bar slowly back to the upper chest under control. Avoid bouncing or leaning back.

## Common Mistakes
- Leaning back too much: this stresses the lower back. Squeeze glutes and abs to keep your torso upright.
- Pressing the bar away from the body: this makes the lift less efficient. Keep the bar path close and vertical.
- Flaring elbows too wide: this can strain the shoulders. Keep elbows slightly in front of the bar at the start.
- Using too much weight: if range of motion or control is lost, reduce the load.

## Resources
- [How To Overhead Press: The Barbell Overhead Press](https://www.youtube.com/watch?v=2yjwXTZQDDI)
- [How to Do the Overhead Press for Upper-Body Size and Strength](https://barbend.com/overhead-press/)', 5, 10),
       (33, 'Barbell Row', 'upper', 'weighted', '## Instructions
1. Stand with feet about hip-width apart and hold a barbell with a shoulder-width overhand grip. Brace your core, soften your knees, and keep your chest lifted.
2. Hinge at your hips until your torso is close to parallel to the floor. Keep your back flat, neck neutral, and let the bar hang under your shoulders.
3. Pull the bar toward your lower ribs or upper waist by driving your elbows back. Squeeze your shoulder blades together at the top without shrugging.
4. Lower the bar slowly until your arms are fully extended. Exhale as you row, inhale as you lower, and avoid using momentum.
5. Perform controlled repetitions, stopping if you cannot keep a flat back or stable torso.

## Common Mistakes
- Rounding the lower back: keep your spine neutral and reduce the weight if you cannot hold position.
- Jerking the bar up: use a smooth pull and controlled lowering instead of swinging.
- Standing too upright: maintain the hip hinge so your back muscles do the work.
- Shrugging the shoulders: keep shoulders down and focus on pulling elbows back.

## Resources
- [How To: Barbell Row](https://www.youtube.com/watch?v=vT2GjY_Umpw)
- [How to Do the Bent-Over Barbell Row for a Bigger, Stronger Back](https://barbend.com/barbell-row/)', 5, 10),
       (34, 'Face Pull', 'upper', 'weighted', '## Instructions
1. Set a cable pulley at upper-chest to face height with a rope attachment. Stand tall, brace your core, and hold the rope with thumbs pointing behind you.
2. Step back until the cable is taut. Keep your chest up, shoulders down, and arms extended in front with a slight bend in the elbows.
3. Pull the rope toward your face by driving your elbows out and back. Separate the rope ends as they approach your forehead, and squeeze your upper back at the end.
4. Pause briefly, then return slowly to the start under control. Exhale as you pull, inhale as you return, and avoid using momentum.

## Common Mistakes
- Using too much weight: This turns the movement into a shrug or lean-back. Lower the weight and keep the motion smooth.
- Letting shoulders rise: Keep shoulders down to avoid neck tension and better target the rear shoulders and upper back.
- Pulling too low: Bring the rope toward the face or upper chest level, not the stomach, to keep the exercise effective.
- Bending the lower back: Brace your core and stay tall instead of arching to finish the rep.

## Resources
- [How To: Face Pull](https://www.youtube.com/watch?v=rep-qVOkqgk)', 5, 10),
       (35, 'Hip Thrust', 'lower', 'weighted', '## Instructions
1. Sit on the floor with your upper back against a bench and a barbell or pad placed across your hips. Bend your knees and plant your feet flat, about hip-width apart.
2. Brace your core, keep your chin slightly tucked, and position your feet so your shins will be nearly vertical at the top. Keep your back supported on the bench edge.
3. Drive through your heels and lift your hips until your torso is parallel to the floor. Squeeze your glutes hard at the top without overarching your lower back.
4. Lower your hips under control until just above the floor, then repeat with a smooth tempo. Exhale as you lift and inhale as you lower.

## Common Mistakes
- Pushing through the toes: this can shift tension away from the glutes; keep pressure through the heels and midfoot.
- Overarching the lower back at the top: lock out with your glutes and ribs down instead of leaning back.
- Poor foot placement: feet too far or too close can reduce power; adjust until shins are nearly vertical at the top.
- Letting the knees cave inward: keep them tracking over your toes throughout the lift.

## Resources
- [How To Do A Barbell Hip Thrust | Glute Exercise Demo](https://www.youtube.com/watch?v=SEdqd1n0cvg)', 5, 10),
       (36, 'Bulgarian Split Squat', 'lower', 'bodyweight', '## Instructions
1. Stand about two feet in front of a bench or box and place the top of one foot behind you on it. Keep your front foot far enough forward so your front knee stays over your mid-foot.
2. Brace your core, keep your chest tall, and square your hips forward. Let your arms hang naturally or place hands on hips for balance.
3. Lower under control by bending your front knee and dropping your back knee toward the floor. Keep most of your weight on the front leg and your front heel planted.
4. Exhale and press through your front heel to return to standing. Move slowly and avoid bouncing at the bottom.
5. Complete all reps on one side, then switch legs. Start with bodyweight before adding dumbbells.

## Common Mistakes
- Front foot too close to the bench: this pushes the knee too far forward; step farther out so you can lower comfortably.
- Leaning too far forward: keep your chest up and core tight to protect balance and your lower back.
- Pushing off the back leg: use the rear leg only for support; the front leg should do most of the work.
- Letting the front knee cave inward: track the knee in line with your toes.

## Resources
- [How To Do Bulgarian Split Squats (Form Tutorial)](https://www.youtube.com/watch?v=2C-uNgKwPLE)
- [How to Do the Bulgarian Split Squat for Leg Size, Strength, and Mobility](https://barbend.com/bulgarian-split-squat/)', 5, 10),
       (37, 'Hammer Curl', 'upper', 'weighted', '## Instructions
1. Stand tall with a dumbbell in each hand at your sides, palms facing inward, feet about hip-width apart, and shoulders relaxed.
2. Keep your elbows close to your ribs, wrists straight, and chest up. Brace your core so your body stays still throughout the lift.
3. Curl the dumbbells upward with a neutral grip until your forearms are nearly vertical. Pause briefly at the top while keeping tension on the biceps and forearms.
4. Exhale as you lift and inhale as you lower. Lower the weights slowly and with control until your arms are fully extended without locking out hard.

## Common Mistakes
- Swinging the weights: using momentum reduces muscle work; keep your torso still and lift under control.
- Letting elbows drift forward: this shifts tension away from the target muscles; keep elbows pinned near your sides.
- Bending the wrists: this can strain the joints; keep wrists neutral and aligned with your forearms.
- Using too much weight: poor form increases injury risk; lower the load and focus on full range of motion.

## Resources
- [How To Do Hammer Curls](https://www.youtube.com/watch?v=zC3nLlEvin4)', 5, 10),
       (38, 'Skull Crusher', 'upper', 'weighted', '## Instructions
1. Lie flat on a bench with your feet planted and hold an EZ bar or dumbbells above your chest with straight arms.
2. Keep your upper arms mostly vertical, elbows tucked in, and wrists neutral before starting the rep.
3. Bend your elbows to lower the weight toward your forehead or just behind your head in a controlled motion.
4. Exhale and extend your elbows to return to the start, stopping just short of locking out hard at the top.

## Common Mistakes
- Letting the elbows flare wide: this shifts stress away from the triceps; keep them pointed forward and fairly narrow.
- Moving the shoulders too much: turning it into a pullover reduces triceps work; keep upper arms steady throughout.
- Lowering the weight too fast: this can strain the elbows; control the descent and avoid bouncing.
- Using too much weight: if your wrists bend or form breaks, reduce the load and focus on clean reps.

## Resources
- [How To: Dumbbell Skull Crusher](https://www.youtube.com/watch?v=l3n6F4eHf9M)', 5, 10),
       (39, 'Hanging Leg Raise', 'upper', 'bodyweight', '## Instructions
1. Hang from a pull-up bar with a shoulder-width overhand grip. Keep your arms straight, shoulders active, and core braced.
2. Start with your legs together and slightly in front of your body. Avoid swinging by tightening your abs and glutes.
3. Raise your legs in a controlled motion until your thighs reach at least hip height, or higher if you can without losing form.
4. Exhale as you lift, then lower your legs slowly while keeping tension in your core. Do not drop them.
5. Perform smooth repetitions and stop if your lower back arches excessively or your grip fails.

## Common Mistakes
- Using momentum to swing the legs: pause at the bottom and lift with your abs and hip flexors instead.
- Shrugging the shoulders toward the ears: keep your shoulders packed down to protect the joints.
- Arching the lower back: brace your core and lift only as high as you can while staying controlled.
- Bending the knees too much unintentionally: keep the legs as straight as your mobility and strength allow.

## Resources
- [How To Do Hanging Leg Raises](https://www.youtube.com/watch?v=Pr1ieGZ5atk)
- [Hanging Leg Raise Exercise Guide](https://www.bodybuilding.com/exercises/hanging-leg-raise)', 5, 10) ON CONFLICT(name) DO
UPDATE SET category = excluded.category,
    exercise_type = excluded.exercise_type,
    description_markdown = excluded.description_markdown,
    rep_min = excluded.rep_min,
    rep_max = excluded.rep_max;

-- Plank is a time-based hold; the main INSERT above leaves it as 'bodyweight'
-- so the schema CHECK doesn't reject the row (time_based requires a non-null
-- default_starting_seconds). This atomic UPDATE flips both columns at once.
UPDATE exercises
SET exercise_type            = 'time_based',
    default_starting_seconds = 30,
    rep_min                  = NULL,
    rep_max                  = NULL
WHERE name = 'Plank';

INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary)
VALUES (1, 'Forearms', 0),
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
       (2, 'Shoulders', 0),
       (2, 'Triceps', 1),
       -- Note: 'Upper Back' was previously listed as a Bench Press secondary, but
       -- the upper back acts as an isometric stabilizer in scapular retraction, not
       -- as a worked muscle. Removed here; prod rows are deleted via the companion
       -- one-shot in docs/.
       (3, 'Shoulders', 0),
       (3, 'Triceps', 1),
       (4, 'Biceps', 1),
       (4, 'Forearms', 0),
       (5, 'Side Delts', 1),
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
       (9, 'Rear Delts', 0),
       (9, 'Upper Back', 1),
       (10, 'Biceps', 1),
       (10, 'Forearms', 0),
       (10, 'Lats', 1),
       (10, 'Rear Delts', 0),
       (10, 'Upper Back', 0),
       (11, 'Biceps', 0),
       (11, 'Lats', 1),
       (11, 'Lower Back', 0),
       (11, 'Rear Delts', 0),
       (11, 'Upper Back', 1),
       (12, 'Biceps', 0),
       (12, 'Forearms', 0),
       (12, 'Lats', 1),
       (12, 'Rear Delts', 0),
       (12, 'Upper Back', 1),
       (13, 'Abs', 1),
       (13, 'Obliques', 0),
       (14, 'Calves', 0),
       (14, 'Glutes', 1),
       (14, 'Hamstrings', 0),
       (14, 'Quads', 1),
       (15, 'Quads', 1),
       (16, 'Calves', 0),
       (16, 'Hamstrings', 1),
       (17, 'Calves', 1),
       (17, 'Quads', 0),
       (18, 'Glutes', 0),
       (18, 'Hamstrings', 0),
       (18, 'Lower Back', 1),
       (22, 'Chest', 1),
       (22, 'Shoulders', 0),
       (22, 'Triceps', 0),
       (22, 'Upper Back', 0),
       (23, 'Glutes', 1),
       (23, 'Hamstrings', 1),
       (23, 'Lower Back', 0),
       (25, 'Glutes', 1),
       (26, 'Adductors', 1),
       (26, 'Glutes', 0),
       (27, 'Obliques', 1),
       (27, 'Abs', 0),
       (28, 'Calves', 1),
       (29, 'Glutes', 1),
       (29, 'Quads', 1),
       (29, 'Hamstrings', 0),
       (29, 'Lower Back', 0),
       (30, 'Chest', 1),
       (30, 'Shoulders', 0),
       (31, 'Glutes', 1),
       (31, 'Quads', 1),
       (31, 'Abs', 0),
       (31, 'Hamstrings', 0),
       (32, 'Shoulders', 1),
       (32, 'Triceps', 1),
       (32, 'Abs', 0),
       (32, 'Upper Back', 0),
       (33, 'Lats', 1),
       (33, 'Upper Back', 1),
       (33, 'Biceps', 0),
       (33, 'Lower Back', 0),
       (33, 'Rear Delts', 0),
       (34, 'Rear Delts', 1),
       (34, 'Upper Back', 1),
       (34, 'Traps', 0),
       (34, 'Triceps', 0),
       (35, 'Glutes', 1),
       (35, 'Hamstrings', 0),
       (35, 'Quads', 0),
       (37, 'Biceps', 1),
       (37, 'Forearms', 1),
       (37, 'Shoulders', 0),
       (37, 'Triceps', 0),
       (38, 'Triceps', 1),
       (38, 'Forearms', 0),
       (38, 'Shoulders', 0),
-- Bodyweight exercises
       -- Push-Up: chest + triceps are the prime movers. Biceps and Lats were
       -- previously listed as primary in some prod rows but are not movers (biceps
       -- has no elbow-flexion load; lats only stabilize the trunk). Those extra
       -- prod rows are deleted via the companion one-shot in docs/.
       (19, 'Chest', 1),
       (19, 'Triceps', 1),
       (19, 'Shoulders', 0),
       (19, 'Abs', 0),
       (19, 'Forearms', 0),
       (19, 'Upper Back', 0),
       -- Ab Wheel Rollout: abs + obliques are the prime movers. Glutes and quads
       -- engage isometrically to keep the body rigid; calves and hamstrings stabilize
       -- the back leg/foot. Listing them as secondary captures the engagement
       -- without inflating primary-set counts for those muscles. The ON CONFLICT
       -- update will demote any rows in prod that were marked primary.
       (20, 'Abs', 1),
       (20, 'Obliques', 1),
       (20, 'Shoulders', 0),
       (20, 'Lats', 0),
       (20, 'Glutes', 0),
       (20, 'Quads', 0),
       (20, 'Calves', 0),
       (20, 'Hamstrings', 0),
       -- Plank: abs are the prime mover; obliques + lower back + glutes + quads
       -- all engage isometrically as stabilizers. The Hip Flexors muscle group
       -- was retired; its prod rows are deleted via the companion one-shot in docs/.
       (21, 'Abs', 1),
       (21, 'Obliques', 0),
       (21, 'Shoulders', 0),
       (21, 'Glutes', 0),
       (21, 'Lower Back', 0),
       (21, 'Quads', 0),
       (36, 'Glutes', 1),
       (36, 'Quads', 1),
       (36, 'Abs', 0),
       (36, 'Hamstrings', 0),
       (39, 'Abs', 1),
       (39, 'Forearms', 0),
       (39, 'Obliques', 0),
-- Assisted exercises
       -- Assisted Pull-Up: lats + upper back are the prime movers (matches the
       -- Pulldown classification). Biceps is a synergist, not a prime mover.
       -- The ON CONFLICT update demotes Biceps and promotes Upper Back in prod.
       (24, 'Lats', 1),
       (24, 'Upper Back', 1),
       (24, 'Biceps', 0),
       (24, 'Rear Delts', 0),
       (24, 'Forearms', 0) ON CONFLICT(exercise_id, muscle_group_name) DO
UPDATE SET is_primary = excluded.is_primary;

INSERT INTO feature_flags (name, enabled)
VALUES ('maintenance_mode', 0) ON CONFLICT(name) DO
UPDATE SET enabled = excluded.enabled;

INSERT INTO muscle_group_weekly_targets (muscle_group_name, weekly_sets_target)
VALUES ('Biceps', 8),
       ('Chest', 10),
       ('Glutes', 8),
       ('Hamstrings', 8),
       ('Lats', 10),
       ('Quads', 10),
       ('Shoulders', 10),
       ('Triceps', 8),
       ('Upper Back', 10) ON CONFLICT (muscle_group_name) DO
UPDATE SET weekly_sets_target = excluded.weekly_sets_target;
