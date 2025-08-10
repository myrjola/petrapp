# Domain Models & Business Logic - Workout Package

Guidelines for working with domain models, business logic, and data access patterns in `internal/workout/`.

## Architecture Overview

### Domain-Driven Design Structure

- **Models** (`models.go`) - Pure domain entities and value objects
- **Service** (`service.go`) - Business logic and workflow orchestration
- **Repository** (`repository.go`) - Data access interfaces and aggregates
- **Generator** (`generator.go`) - Workout generation algorithm

### Separation of Concerns

- **Domain models** represent business concepts (Exercise, Session, Set)
- **Repository aggregates** represent data persistence structure
- **Service layer** coordinates between domain and repository, handles business rules
- **Generators** implement complex algorithms (workout creation, progression)

## Domain Models

### Core Entities

```go
type Exercise struct {
    ID                    int
    Name                  string
    Category              Category     // full_body, upper, lower
    ExerciseType          ExerciseType // weighted, bodyweight
    DescriptionMarkdown   string
    PrimaryMuscleGroups   []string
    SecondaryMuscleGroups []string
}

type Session struct {
    Date             time.Time
    DifficultyRating *int        // 1-5 user feedback
    StartedAt        time.Time
    CompletedAt      time.Time
    ExerciseSets     []ExerciseSet
}

type ExerciseSet struct {
    Exercise          Exercise
    Sets              []Set
    WarmupCompletedAt *time.Time
}

type Set struct {
    WeightKg      *float64   // Nullable for bodyweight exercises
    MinReps       int
    MaxReps       int
    CompletedReps *int       // Actual reps completed
    CompletedAt   *time.Time // When set was completed
}
```

### Strongly Typed Enums

Always use typed constants with exhaustive validation:

```go
type Category string

const (
    CategoryFullBody Category = "full_body"
    CategoryUpper    Category = "upper"  
    CategoryLower    Category = "lower"
)

// Validate enum values in constructors/setters
func (c Category) IsValid() bool {
    switch c {
    case CategoryFullBody, CategoryUpper, CategoryLower:
        return true
    default:
        return false
    }
}
```

### Model Validation Patterns

- Use pointer types for nullable fields (`*int`, `*time.Time`)
- Implement validation methods on domain models
- Use builder patterns for complex model construction
- Keep models focused on data and basic validation, not business logic

## Service Layer Patterns

### Business Logic Organization

```go
type Service struct {
    repo         *repository
    logger       *slog.Logger
    // Dependencies injected via constructor
}

// Public API methods handle business workflows
func (s *Service) StartWorkout(ctx context.Context, date time.Time) error {
    // 1. Validate business rules
    // 2. Coordinate repository operations
    // 3. Handle domain events
    return s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
        sess.StartedAt = time.Now()
        return true, nil // true = persist changes
    })
}
```

### Error Handling

- Use sentinel errors for business conditions: `var ErrNotFound = sql.ErrNoRows`
- Wrap errors with context: `fmt.Errorf("operation description: %w", err)`
- Check for specific errors using `errors.Is(err, ErrNotFound)`
- Let service layer handle business validation, repository handles data access

### Context Propagation

- Always pass `context.Context` as first parameter
- Use context for cancellation, timeouts, and request-scoped values
- Pass context down to repository methods

## Repository Interface Patterns

### Aggregate-Based Design

```go
// Repository aggregates represent data persistence structure
type sessionAggregate struct {
    Date             time.Time
    DifficultyRating *int
    StartedAt        time.Time
    CompletedAt      time.Time
    ExerciseSets     []exerciseSetAggregate // Nested aggregates
}

// Repository interfaces define data access contracts
type sessionRepository interface {
    List(ctx context.Context, sinceDate time.Time) ([]sessionAggregate, error)
    Get(ctx context.Context, date time.Time) (sessionAggregate, error)
    Create(ctx context.Context, sess sessionAggregate) error
    Update(ctx context.Context, date time.Time, updateFn func(*sessionAggregate) (bool, error)) error
}
```

### Update Pattern with Function Parameter

Use functional updates for transactional safety:

```go
// Repository method accepts update function
func (r *sessionRepositoryImpl) Update(ctx context.Context, date time.Time, updateFn func(*sessionAggregate) (bool, error)) error {
    // 1. Load current state
    // 2. Call update function
    // 3. Save if function returns true
    // 4. Handle concurrency/consistency
}

// Service calls with lambda
err := s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
    if sess.CompletedAt != nil {
        return false, errors.New("workout already completed")
    }
    sess.CompletedAt = time.Now()
    return true, nil
})
```

## Data Conversion Patterns

### Domain ↔ Aggregate Conversion

```go
// Convert repository aggregate to domain model
func (r *repository) aggregateToDomain(agg sessionAggregate, exercises []Exercise) (Session, error) {
    session := Session{
        Date:             agg.Date,
        DifficultyRating: agg.DifficultyRating,
        StartedAt:        agg.StartedAt,
        CompletedAt:      agg.CompletedAt,
    }
    
    // Build exercise sets with domain exercises
    for _, setAgg := range agg.ExerciseSets {
        exercise := findExerciseByID(exercises, setAgg.ExerciseID)
        session.ExerciseSets = append(session.ExerciseSets, ExerciseSet{
            Exercise:          exercise,
            Sets:              setAgg.Sets,
            WarmupCompletedAt: setAgg.WarmupCompletedAt,
        })
    }
    
    return session, nil
}
```

### Conversion Guidelines

- Keep conversion logic in repository layer, not service layer
- Handle missing data gracefully (use zero values or return errors)
- Validate converted data before returning to service layer
- Use helper functions for complex nested conversions

## Integration with Other Layers

### Service ↔ Handler Integration

- Handlers call service methods with validated input
- Service returns domain models to handlers
- Handlers convert domain models to template data structures
- Service handles all business validation and coordination

### Repository ↔ Database Integration

- Repository implementations handle SQL queries and transactions
- Convert between Go types and database types (time.Time ↔ string, etc.)
- Handle database-specific constraints and validations
- Use proper foreign key relationships and cascading deletes

## Common Patterns and Anti-Patterns

### ✅ Good Patterns

- Inject dependencies through constructors
- Use interfaces for testability and flexibility
- Keep domain models focused on data and basic validation
- Use functional updates for transactional safety
- Separate concerns between domain, service, and repository layers

### ❌ Anti-Patterns

- Don't put database logic in domain models
- Don't put business logic in repository implementations
- Don't use global variables or singletons
- Don't bypass service layer from handlers
- Don't mix domain and repository aggregate types

## Error Handling Strategies

### Business Rule Violations

```go
// Define sentinel errors for business conditions
var (
    ErrWorkoutAlreadyStarted   = errors.New("workout already started")
    ErrWorkoutNotFound        = errors.New("workout not found")
    ErrInvalidDifficultyRating = errors.New("difficulty rating must be 1-5")
)

// Check for specific errors in handlers
if errors.Is(err, ErrWorkoutNotFound) {
    // Handle not found case specifically
}
```

### Validation and Input Sanitization

- Validate at service layer before calling repository
- Use domain model validation methods for business rules
- Sanitize and normalize input data consistently
- Return meaningful error messages for user-facing errors
