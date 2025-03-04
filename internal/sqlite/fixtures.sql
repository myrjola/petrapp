-- Full Body Exercises (ID range 1-100)
INSERT OR REPLACE INTO exercises (id, name, category) VALUES
    (1, 'Barbell Squat', 'full_body'),
    (2, 'Deadlift', 'full_body'),
    (3, 'Power Clean', 'full_body'),
    (4, 'Turkish Get-Up', 'full_body'),
    (5, 'Kettlebell Swing', 'full_body');

-- Upper Body Exercises (ID range 101-200)
INSERT OR REPLACE INTO exercises (id, name, category) VALUES
    (101, 'Bench Press', 'upper'),
    (102, 'Overhead Press', 'upper'),
    (103, 'Pull-Up', 'upper'),
    (104, 'Bent Over Row', 'upper'),
    (105, 'Dumbbell Row', 'upper'),
    (106, 'Push-Up', 'upper'),
    (107, 'Dips', 'upper'),
    (108, 'Face Pull', 'upper'),
    (109, 'Lateral Raise', 'upper'),
    (110, 'Tricep Extension', 'upper'),
    (111, 'Bicep Curl', 'upper');

-- Lower Body Exercises (ID range 201-300)
INSERT OR REPLACE INTO exercises (id, name, category) VALUES
    (201, 'Romanian Deadlift', 'lower'),
    (202, 'Front Squat', 'lower'),
    (203, 'Leg Press', 'lower'),
    (204, 'Bulgarian Split Squat', 'lower'),
    (205, 'Calf Raise', 'lower'),
    (206, 'Hip Thrust', 'lower'),
    (207, 'Leg Extension', 'lower'),
    (208, 'Leg Curl', 'lower'),
    (209, 'Walking Lunge', 'lower');
