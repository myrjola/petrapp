-- =============================
-- Muscle Groups
-- =============================
INSERT
OR REPLACE INTO muscle_groups (name)
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
('Adductors');

-- =============================
-- Upper Body Exercises (ID range 1000-1999)
-- =============================
INSERT
OR REPLACE INTO exercises (id, name, category) VALUES
-- Chest Dominant
(1000, 'Bench Press', 'upper'),
(1001, 'Incline Bench Press', 'upper'),
(1002, 'Decline Bench Press', 'upper'),
(1003, 'Dumbbell Bench Press', 'upper'),
(1004, 'Incline Dumbbell Press', 'upper'),
(1005, 'Push-ups', 'upper'),
(1006, 'Dips (Chest Focus)', 'upper'),
(1007, 'Cable Fly', 'upper'),
(1008, 'Dumbbell Fly', 'upper'),
(1009, 'Machine Chest Press', 'upper'),

-- Shoulder Dominant
(1100, 'Overhead Press', 'upper'),
(1101, 'Seated Dumbbell Press', 'upper'),
(1102, 'Arnold Press', 'upper'),
(1103, 'Lateral Raise', 'upper'),
(1104, 'Front Raise', 'upper'),
(1105, 'Reverse Fly', 'upper'),
(1106, 'Face Pull', 'upper'),
(1107, 'Upright Row', 'upper'),
(1108, 'Cable Lateral Raise', 'upper'),
(1109, 'Military Press', 'upper'),

-- Back Dominant
(1200, 'Pull-up', 'upper'),
(1201, 'Chin-up', 'upper'),
(1202, 'Lat Pulldown', 'upper'),
(1203, 'Seated Row', 'upper'),
(1204, 'Bent Over Row', 'upper'),
(1205, 'T-Bar Row', 'upper'),
(1206, 'Single-Arm Dumbbell Row', 'upper'),
(1207, 'Inverted Row', 'upper'),
(1208, 'Straight-Arm Pulldown', 'upper'),
(1209, 'Shrugs', 'upper'),

-- Arm Dominant
(1300, 'Bicep Curl', 'upper'),
(1301, 'Hammer Curl', 'upper'),
(1302, 'Preacher Curl', 'upper'),
(1303, 'Concentration Curl', 'upper'),
(1304, 'Tricep Pushdown', 'upper'),
(1305, 'Skull Crusher', 'upper'),
(1306, 'Overhead Tricep Extension', 'upper'),
(1307, 'Close-Grip Bench Press', 'upper'),
(1308, 'Cable Curl', 'upper'),
(1309, 'Reverse Curl', 'upper');

-- =============================
-- Lower Body Exercises (ID range 2000-2999)
-- =============================
INSERT
OR REPLACE INTO exercises (id, name, category) VALUES
-- Quad Dominant
(2000, 'Back Squat', 'lower'),
(2001, 'Front Squat', 'lower'),
(2002, 'Leg Press', 'lower'),
(2003, 'Leg Extension', 'lower'),
(2004, 'Hack Squat', 'lower'),
(2005, 'Bulgarian Split Squat', 'lower'),
(2006, 'Sissy Squat', 'lower'),
(2007, 'Walking Lunge', 'lower'),
(2008, 'Goblet Squat', 'lower'),
(2009, 'Step Up', 'lower'),

-- Hamstring/Glute Dominant
(2100, 'Romanian Deadlift', 'lower'),
(2101, 'Leg Curl', 'lower'),
(2102, 'Good Morning', 'lower'),
(2103, 'Glute Bridge', 'lower'),
(2104, 'Hip Thrust', 'lower'),
(2105, 'Reverse Hyper', 'lower'),
(2106, 'Cable Pull Through', 'lower'),
(2107, 'Kettlebell Swing', 'lower'),
(2108, 'Nordic Hamstring Curl', 'lower'),
(2109, 'Glute-Ham Raise', 'lower'),

-- Calf/Accessory
(2200, 'Standing Calf Raise', 'lower'),
(2201, 'Seated Calf Raise', 'lower'),
(2202, 'Donkey Calf Raise', 'lower'),
(2203, 'Adductor Machine', 'lower'),
(2204, 'Abductor Machine', 'lower'),
(2205, 'Side Lunge', 'lower'),
(2206, 'Calf Press on Leg Press', 'lower'),
(2207, 'Single-Leg Calf Raise', 'lower'),
(2208, 'Sumo Squat', 'lower'),
(2209, 'Lateral Lunge', 'lower');

-- =============================
-- Full Body Exercises (ID range 3000-3999)
-- =============================
INSERT
OR REPLACE INTO exercises (id, name, category) VALUES
-- Olympic/Compound
(3000, 'Conventional Deadlift', 'full_body'),
(3001, 'Clean and Jerk', 'full_body'),
(3002, 'Snatch', 'full_body'),
(3003, 'Clean', 'full_body'),
(3004, 'Push Press', 'full_body'),
(3005, 'Thruster', 'full_body'),
(3006, 'Turkish Get-Up', 'full_body'),
(3007, 'Sumo Deadlift', 'full_body'),
(3008, 'Hex Bar Deadlift', 'full_body'),
(3009, 'Clean and Press', 'full_body'),

-- Functional/Crossfit
(3100, 'Burpee', 'full_body'),
(3101, 'Medicine Ball Slam', 'full_body'),
(3102, 'Battle Rope Exercise', 'full_body'),
(3103, 'Box Jump', 'full_body'),
(3104, 'Prowler Push', 'full_body'),
(3105, 'Tire Flip', 'full_body'),
(3106, 'Farmer''s Walk', 'full_body'),
(3107, 'Sled Drag', 'full_body'),
(3108, 'Wall Ball', 'full_body'),
(3109, 'Renegade Row', 'full_body'),

-- Core-Focused Full Body
(3200, 'Plank', 'full_body'),
(3201, 'Russian Twist', 'full_body'),
(3202, 'Mountain Climber', 'full_body'),
(3203, 'Ab Wheel Rollout', 'full_body'),
(3204, 'Hanging Leg Raise', 'full_body'),
(3205, 'Medicine Ball Woodchop', 'full_body'),
(3206, 'Bird Dog', 'full_body'),
(3207, 'Dead Bug', 'full_body'),
(3208, 'Hollow Hold', 'full_body'),
(3209, 'Dragon Flag', 'full_body');

-- =============================
-- Exercise Muscle Group Relationships
-- =============================
INSERT OR REPLACE INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary) VALUES
-- Bench Press
(1000, 'Chest', 1),
(1000, 'Shoulders', 0),
(1000, 'Triceps', 0),

-- Incline Bench Press
(1001, 'Chest', 1),
(1001, 'Shoulders', 1),
(1001, 'Triceps', 0),

-- Decline Bench Press
(1002, 'Chest', 1),
(1002, 'Triceps', 0),

-- Dumbbell Bench Press
(1003, 'Chest', 1),
(1003, 'Shoulders', 0),
(1003, 'Triceps', 0),

-- Incline Dumbbell Press
(1004, 'Chest', 1),
(1004, 'Shoulders', 1),
(1004, 'Triceps', 0),

-- Push-ups
(1005, 'Chest', 1),
(1005, 'Shoulders', 0),
(1005, 'Triceps', 0),
(1005, 'Abs', 0),

-- Dips (Chest Focus)
(1006, 'Chest', 1),
(1006, 'Triceps', 1),
(1006, 'Shoulders', 0),

-- Cable Fly
(1007, 'Chest', 1),

-- Dumbbell Fly
(1008, 'Chest', 1),

-- Machine Chest Press
(1009, 'Chest', 1),
(1009, 'Triceps', 0),
(1009, 'Shoulders', 0),

-- Overhead Press
(1100, 'Shoulders', 1),
(1100, 'Triceps', 0),
(1100, 'Upper Back', 0),

-- Seated Dumbbell Press
(1101, 'Shoulders', 1),
(1101, 'Triceps', 0),

-- Arnold Press
(1102, 'Shoulders', 1),
(1102, 'Triceps', 0),

-- Lateral Raise
(1103, 'Shoulders', 1),

-- Front Raise
(1104, 'Shoulders', 1),

-- Reverse Fly
(1105, 'Upper Back', 1),
(1105, 'Shoulders', 0),

-- Face Pull
(1106, 'Upper Back', 1),
(1106, 'Shoulders', 1),

-- Upright Row
(1107, 'Shoulders', 1),
(1107, 'Traps', 0),

-- Cable Lateral Raise
(1108, 'Shoulders', 1),

-- Military Press
(1109, 'Shoulders', 1),
(1109, 'Triceps', 0),

-- Pull-up
(1200, 'Lats', 1),
(1200, 'Upper Back', 0),
(1200, 'Biceps', 0),
(1200, 'Forearms', 0),

-- Chin-up
(1201, 'Lats', 1),
(1201, 'Biceps', 1),
(1201, 'Upper Back', 0),
(1201, 'Forearms', 0),

-- Lat Pulldown
(1202, 'Lats', 1),
(1202, 'Upper Back', 0),
(1202, 'Biceps', 0),

-- Seated Row
(1203, 'Upper Back', 1),
(1203, 'Lats', 0),
(1203, 'Biceps', 0),

-- Bent Over Row
(1204, 'Upper Back', 1),
(1204, 'Lats', 0),
(1204, 'Biceps', 0),
(1204, 'Lower Back', 0),

-- T-Bar Row
(1205, 'Upper Back', 1),
(1205, 'Lats', 0),
(1205, 'Biceps', 0),

-- Single-Arm Dumbbell Row
(1206, 'Upper Back', 1),
(1206, 'Lats', 0),
(1206, 'Biceps', 0),

-- Inverted Row
(1207, 'Upper Back', 1),
(1207, 'Biceps', 0),

-- Straight-Arm Pulldown
(1208, 'Lats', 1),
(1208, 'Abs', 0),

-- Shrugs
(1209, 'Traps', 1),

-- Bicep Curl
(1300, 'Biceps', 1),
(1300, 'Forearms', 0),

-- Hammer Curl
(1301, 'Biceps', 1),
(1301, 'Forearms', 1),

-- Preacher Curl
(1302, 'Biceps', 1),

-- Concentration Curl
(1303, 'Biceps', 1),

-- Tricep Pushdown
(1304, 'Triceps', 1),

-- Skull Crusher
(1305, 'Triceps', 1),

-- Overhead Tricep Extension
(1306, 'Triceps', 1),

-- Close-Grip Bench Press
(1307, 'Triceps', 1),
(1307, 'Chest', 0),
(1307, 'Shoulders', 0),

-- Cable Curl
(1308, 'Biceps', 1),

-- Reverse Curl
(1309, 'Forearms', 1),
(1309, 'Biceps', 0),

-- Back Squat
(2000, 'Quads', 1),
(2000, 'Glutes', 1),
(2000, 'Lower Back', 0),
(2000, 'Hamstrings', 0),
(2000, 'Abs', 0),

-- Front Squat
(2001, 'Quads', 1),
(2001, 'Abs', 0),
(2001, 'Upper Back', 0),

-- Leg Press
(2002, 'Quads', 1),
(2002, 'Glutes', 0),
(2002, 'Hamstrings', 0),

-- Leg Extension
(2003, 'Quads', 1),

-- Hack Squat
(2004, 'Quads', 1),
(2004, 'Glutes', 0),

-- Bulgarian Split Squat
(2005, 'Quads', 1),
(2005, 'Glutes', 1),
(2005, 'Hamstrings', 0),

-- Sissy Squat
(2006, 'Quads', 1),

-- Walking Lunge
(2007, 'Quads', 1),
(2007, 'Glutes', 1),
(2007, 'Hamstrings', 0),

-- Goblet Squat
(2008, 'Quads', 1),
(2008, 'Glutes', 0),
(2008, 'Abs', 0),

-- Step Up
(2009, 'Quads', 1),
(2009, 'Glutes', 0),

-- Romanian Deadlift
(2100, 'Hamstrings', 1),
(2100, 'Glutes', 1),
(2100, 'Lower Back', 0),

-- Leg Curl
(2101, 'Hamstrings', 1),

-- Good Morning
(2102, 'Hamstrings', 1),
(2102, 'Lower Back', 1),
(2102, 'Glutes', 0),

-- Glute Bridge
(2103, 'Glutes', 1),
(2103, 'Hamstrings', 0),

-- Hip Thrust
(2104, 'Glutes', 1),
(2104, 'Hamstrings', 0),

-- Reverse Hyper
(2105, 'Glutes', 1),
(2105, 'Hamstrings', 1),
(2105, 'Lower Back', 0),

-- Cable Pull Through
(2106, 'Glutes', 1),
(2106, 'Hamstrings', 0),

-- Kettlebell Swing
(2107, 'Glutes', 1),
(2107, 'Hamstrings', 0),
(2107, 'Lower Back', 0),
(2107, 'Shoulders', 0),

-- Nordic Hamstring Curl
(2108, 'Hamstrings', 1),

-- Glute-Ham Raise
(2109, 'Hamstrings', 1),
(2109, 'Glutes', 1),
(2109, 'Lower Back', 0),

-- Standing Calf Raise
(2200, 'Calves', 1),

-- Seated Calf Raise
(2201, 'Calves', 1),

-- Donkey Calf Raise
(2202, 'Calves', 1),

-- Adductor Machine
(2203, 'Adductors', 1),

-- Abductor Machine
(2204, 'Glutes', 1),
(2204, 'Hip Flexors', 0),

-- Side Lunge
(2205, 'Adductors', 1),
(2205, 'Quads', 0),
(2205, 'Glutes', 0),

-- Calf Press on Leg Press
(2206, 'Calves', 1),

-- Single-Leg Calf Raise
(2207, 'Calves', 1),

-- Sumo Squat
(2208, 'Quads', 1),
(2208, 'Adductors', 1),
(2208, 'Glutes', 0),

-- Lateral Lunge
(2209, 'Quads', 1),
(2209, 'Adductors', 1),
(2209, 'Glutes', 0),

-- Conventional Deadlift
(3000, 'Lower Back', 1),
(3000, 'Hamstrings', 1),
(3000, 'Glutes', 1),
(3000, 'Quads', 0),
(3000, 'Traps', 0),
(3000, 'Forearms', 0),

-- Clean and Jerk
(3001, 'Quads', 1),
(3001, 'Glutes', 1),
(3001, 'Shoulders', 1),
(3001, 'Triceps', 0),
(3001, 'Upper Back', 0),
(3001, 'Hamstrings', 0),
(3001, 'Abs', 0),

-- Snatch
(3002, 'Quads', 1),
(3002, 'Glutes', 1),
(3002, 'Shoulders', 1),
(3002, 'Triceps', 0),
(3002, 'Upper Back', 0),
(3002, 'Hamstrings', 0),
(3002, 'Abs', 0),

-- Clean
(3003, 'Quads', 1),
(3003, 'Glutes', 1),
(3003, 'Upper Back', 0),
(3003, 'Hamstrings', 0),
(3003, 'Traps', 0),
(3003, 'Abs', 0),

-- Push Press
(3004, 'Shoulders', 1),
(3004, 'Triceps', 0),
(3004, 'Quads', 0),
(3004, 'Abs', 0),

-- Thruster
(3005, 'Quads', 1),
(3005, 'Glutes', 1),
(3005, 'Shoulders', 1),
(3005, 'Triceps', 0),

-- Turkish Get-Up
(3006, 'Shoulders', 1),
(3006, 'Abs', 1),
(3006, 'Glutes', 0),
(3006, 'Quads', 0),
(3006, 'Hamstrings', 0),

-- Sumo Deadlift
(3007, 'Hamstrings', 1),
(3007, 'Glutes', 1),
(3007, 'Quads', 1),
(3007, 'Adductors', 0),
(3007, 'Lower Back', 0),

-- Hex Bar Deadlift
(3008, 'Quads', 1),
(3008, 'Glutes', 1),
(3008, 'Hamstrings', 1),
(3008, 'Lower Back', 0),
(3008, 'Traps', 0),

-- Clean and Press
(3009, 'Quads', 1),
(3009, 'Glutes', 1),
(3009, 'Shoulders', 1),
(3009, 'Triceps', 0),
(3009, 'Upper Back', 0),

-- Burpee
(3100, 'Quads', 1),
(3100, 'Chest', 0),
(3100, 'Shoulders', 0),
(3100, 'Triceps', 0),
(3100, 'Abs', 0),

-- Medicine Ball Slam
(3101, 'Abs', 1),
(3101, 'Shoulders', 0),
(3101, 'Lats', 0),
(3101, 'Quads', 0),

-- Battle Rope Exercise
(3102, 'Shoulders', 1),
(3102, 'Abs', 0),
(3102, 'Upper Back', 0),
(3102, 'Forearms', 0),

-- Box Jump
(3103, 'Quads', 1),
(3103, 'Glutes', 1),
(3103, 'Calves', 0),

-- Prowler Push
(3104, 'Quads', 1),
(3104, 'Glutes', 1),
(3104, 'Calves', 0),
(3104, 'Shoulders', 0),
(3104, 'Abs', 0),

-- Tire Flip
(3105, 'Lower Back', 1),
(3105, 'Quads', 1),
(3105, 'Glutes', 1),
(3105, 'Shoulders', 0),
(3105, 'Biceps', 0),

-- Farmer's Walk
(3106, 'Forearms', 1),
(3106, 'Traps', 1),
(3106, 'Abs', 0),
(3106, 'Quads', 0),
(3106, 'Calves', 0),

-- Sled Drag
(3107, 'Quads', 1),
(3107, 'Glutes', 1),
(3107, 'Hamstrings', 0),
(3107, 'Lower Back', 0),

-- Wall Ball
(3108, 'Quads', 1),
(3108, 'Shoulders', 1),
(3108, 'Triceps', 0),
(3108, 'Abs', 0),

-- Renegade Row
(3109, 'Upper Back', 1),
(3109, 'Abs', 1),
(3109, 'Shoulders', 0),
(3109, 'Biceps', 0),

-- Plank
(3200, 'Abs', 1),
(3200, 'Shoulders', 0),

-- Russian Twist
(3201, 'Abs', 1),
(3201, 'Obliques', 1),

-- Mountain Climber
(3202, 'Abs', 1),
(3202, 'Hip Flexors', 0),
(3202, 'Shoulders', 0),

-- Ab Wheel Rollout
(3203, 'Abs', 1),
(3203, 'Shoulders', 0),
(3203, 'Lats', 0),

-- Hanging Leg Raise
(3204, 'Abs', 1),
(3204, 'Hip Flexors', 0),

-- Medicine Ball Woodchop
(3205, 'Abs', 1),
(3205, 'Obliques', 1),

-- Bird Dog
(3206, 'Lower Back', 1),
(3206, 'Abs', 0),
(3206, 'Glutes', 0),

-- Dead Bug
(3207, 'Abs', 1),
(3207, 'Lower Back', 0),

-- Hollow Hold
(3208, 'Abs', 1),
(3208, 'Hip Flexors', 0),

-- Dragon Flag
(3209, 'Abs', 1),
(3209, 'Lower Back', 0);
