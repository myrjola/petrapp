# Workout Generation Approach

## Overview

The workout generator uses a combination of user input, historical data, and progression models to create personalized
workout plans. The goal is to provide a balanced and challenging workout experience that adapts to the user's fitness
level and feedback. A rough outline of the algorithm is as follows

1. Determine type of workout, e.g., upper body, lower body, full body. This filters the available exercises.
2. Exercise selection based on user history and balanced muscle group targeting.
3. For each exercise, set rep and weight progression based progression model and user feedback.

## Workout Type Determination

```
If today and tomorrow is planned workout:
    Use lower body workout
Else if yesterday was a workout:
    Use upper body workout
Else:
    Use full body workout
```

## Exercise Selection

Exercises target primary and secondary muscle groups. Compound movement is defined as targeting two or more primary
muscle groups.

Exercise Continuity: Repeat some exercises from the previous week's same weekday workout
Benefits: Helps track progress, builds movement proficiency, creates familiarity
Suggestion: Maintain ~80% exercise continuity week-to-week

Keep compound movements more consistent (e.g., deadlift, squat, and bench variations).
Rotate isolation exercises more frequently for variety.

## Set Rep and Weight Progression Models

### If exercise does not have previous history.

* Start with 3 sets of 8 reps at empty weight value so that the user can select a weight that is challenging but doable
  for 8 reps.

### For beginners (1-3 months): Linear progression

* If all sets are completed successfully: Increase weight by 2.5-5kg next workout.
* If sets are partially complete: Keep weight the same.
* If sets are failed: Reduce weight by 5-10%.

### For rest of users: Undulating periodization

Alternate between strength (3-6 reps), hypertrophy (8-12 reps), and endurance (12-15 reps) workouts.

Progress to next workout type when user completes all sets at maximum of rep range for 2 consecutive workouts.

### User Feedback Integration

Processing the 1-5 Scale Feedback:

* 1 (Too Easy): Increase intensity more aggressively next workout
* 2-4 (Optimal Challenge): Follow standard progression
* 5 (Too Difficult): Reduce volume or intensity for next workout
