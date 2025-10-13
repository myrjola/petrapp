# User Added Exercises - Feature Specification

## Overview

This feature allows users to create their own custom exercises that are private to them and not visible to other users. Users can add these custom exercises to their workouts alongside the system-provided exercises.

## Requirements

### Functional Requirements

1. **Exercise Creation**
   - Users can create new exercises with the same attributes as system exercises:
     - Name (required, max 124 characters)
     - Category (full_body, upper, lower)
     - Exercise type (weighted, bodyweight)
     - Description (markdown format, max 20000 characters)
     - Primary muscle groups (optional, from predefined list)
     - Secondary muscle groups (optional, from predefined list)
   - Exercise creation should support both manual entry and AI-assisted generation (existing GenerateExercise flow)

2. **Exercise Visibility**
   - System exercises (pre-populated in fixtures) are visible to all users
   - User-created exercises are only visible to their creator
   - Users cannot see exercises created by other users

3. **Exercise Usage**
   - User-created exercises can be used in workouts exactly like system exercises
   - User-created exercises can be swapped, added, removed from workouts
   - Historical data (weights, reps) for user exercises is tracked per user

4. **Exercise Management**
   - Users can edit their own exercises
   - Users can view detailed information about their exercises
   - System exercises cannot be edited by regular users (admin-only)

### Non-Functional Requirements

1. **Data Integrity**
   - Foreign key relationships must be maintained
   - Deleting a user must cascade-delete their custom exercises
   - Workout sessions must handle both system and user exercises seamlessly

2. **Performance**
   - Exercise list queries should be efficient (single query preferred)
   - No N+1 query problems when loading exercises with muscle groups

3. **Security**
   - Users cannot access or modify exercises they don't own
   - API endpoints must validate exercise ownership
   - Data export must include user's custom exercises

## Design Decision: Shared Table vs. Separate Table

After analyzing the codebase, I recommend using a **shared table approach** with a nullable `user_id` column.

### Approach 1: Shared Table (RECOMMENDED)

Add a nullable `user_id` column to the existing `exercises` table:

```sql
ALTER TABLE exercises ADD COLUMN user_id INTEGER REFERENCES users (id) ON DELETE CASCADE;
```

**Advantages:**
1. **Minimal code changes**: Existing queries work with minor WHERE clause additions
2. **Unified exercise model**: No need for separate domain types or repository methods
3. **Simpler foreign keys**: `workout_exercise` and `exercise_sets` tables don't need changes
4. **Better performance**: Single table queries are faster than JOINs or UNIONs
5. **Easier maintenance**: One table schema, one set of indexes, one migration path
6. **Natural NULL semantics**: NULL user_id = system exercise, works perfectly with SQL

**Disadvantages:**
1. **Schema ambiguity**: NULL user_id convention must be documented
2. **Query complexity**: Most queries need `WHERE user_id IS NULL OR user_id = ?`
3. **Index overhead**: Additional index on user_id column

**Changes Required:**
- Schema: Add nullable `user_id` column with foreign key and index
- Repository: Update List() query to filter by user context
- Repository: Update Create() to accept user_id parameter
- Service: Pass user_id from context when creating exercises
- Tests: Update fixtures to ensure NULL user_id for system exercises

### Approach 2: Separate Table (NOT RECOMMENDED)

Create a new `user_exercises` table with identical structure to `exercises`.

**Advantages:**
1. **Clear separation**: System vs user exercises are explicitly different tables
2. **No NULL handling**: User exercises always have user_id
3. **Independent evolution**: Could add user-specific fields later without affecting system exercises

**Disadvantages:**
1. **Significant code duplication**: Need separate repository methods, models, or complex abstractions
2. **Complex foreign keys**: `workout_exercise` and `exercise_sets` would need polymorphic references or separate user_exercise_id columns
3. **Query complexity**: Need UNION queries to get all exercises for a user
4. **More migrations**: Two tables to maintain and migrate
5. **Harder to ensure consistency**: Duplicate schema means double the maintenance burden
6. **Worse performance**: UNION queries are slower than filtered single-table queries

**Changes Required:**
- Schema: New user_exercises table with same structure as exercises
- Schema: Update workout_exercise and exercise_sets with additional FK columns
- Repository: Separate methods or complex abstraction for user exercises
- Service: Logic to decide which table to query/update
- Tests: Duplicate test fixtures and test cases

### Recommendation: Shared Table

The shared table approach is strongly recommended because:

1. **Least intrusive**: Aligns with the requirement to add this feature with minimal disruption
2. **Leverages existing code**: 90% of current code can be reused with small modifications
3. **Better performance**: Single indexed query vs. UNION or multiple queries
4. **Simpler to understand**: The codebase uses nullable foreign keys elsewhere (see `workout_sessions.difficulty_rating`)
5. **Easier to test**: Existing tests mostly continue to work with minor adjustments
6. **Future-proof**: If we later need user-specific fields, we can add nullable columns

## Data Model (Shared Table Approach)

### Updated Schema

```sql
-- Add user_id column to exercises table
ALTER TABLE exercises
ADD COLUMN user_id INTEGER REFERENCES users (id) ON DELETE CASCADE;

-- Add index for efficient user exercise queries
CREATE INDEX exercises_user_id_idx ON exercises (user_id);

-- Remove the existing UNIQUE constraint on name and add composite constraint
-- This allows multiple users to create exercises with the same name
DROP INDEX IF EXISTS sqlite_autoindex_exercises_1;
ALTER TABLE exercises DROP CONSTRAINT IF EXISTS name;
CREATE UNIQUE INDEX exercises_name_user_id_unique ON exercises (name, user_id);

-- System exercises have NULL user_id
-- User exercises have their creator's user_id
```

### Exercise Ownership Rules

| user_id Value | Meaning | Visible To | Editable By |
|--------------|---------|------------|-------------|
| NULL | System exercise | All users | Admins only |
| <user_id> | User exercise | That user only | That user only |

### Query Patterns

```sql
-- Get all exercises for a user (system + their own)
SELECT * FROM exercises
WHERE user_id IS NULL OR user_id = ?
ORDER BY id;

-- Get only system exercises
SELECT * FROM exercises
WHERE user_id IS NULL
ORDER BY id;

-- Get only a user's exercises
SELECT * FROM exercises
WHERE user_id = ?
ORDER BY id;

-- Check if user owns an exercise
SELECT COUNT(*) FROM exercises
WHERE id = ? AND user_id = ?;
```

## User Interface

### Exercise List View
- Show system exercises and user exercises together
- Visually distinguish user exercises (e.g., badge or icon)
- Add "Create Exercise" button/link

### Exercise Creation Flow
1. User enters exercise name
2. System attempts AI generation (if OpenAI API key configured)
3. User reviews and edits generated exercise details
4. User saves exercise (stored with their user_id)

### Exercise Detail View
- Show "Edit" button only for exercises user owns
- For system exercises, show read-only view

### Workout Exercise Selector
- List shows system exercises + user's own exercises
- User exercises marked with visual indicator
- Filtering/search works across both types

## Security Considerations

1. **Authorization**: All exercise CRUD operations must verify ownership
2. **Data Isolation**: User queries must filter by user_id to prevent data leaks
3. **GDPR Compliance**: User data export must include their custom exercises
4. **Context Validation**: Always extract user_id from authenticated context, never from request parameters

### Repository Layer Security Patterns

**Error Handling for Authorization Failures:**
- Repository methods return `ErrNotFound` for both non-existent exercises and unauthorized access attempts
- This prevents user enumeration attacks (attacker cannot distinguish between "exercise doesn't exist" vs "exercise exists but you can't access it")
- This is an intentional security design decision to avoid information leakage

**Unauthenticated User Handling:**
- `getUserIDFromContext()` returns 0 (zero) for unauthenticated requests
- Queries like `WHERE user_id IS NULL OR user_id = ?` with value 0 will only return system exercises
- Since no user can have user_id = 0 (SQLite INTEGER PRIMARY KEY starts at 1), this safely restricts unauthenticated users to system exercises only

### Input Sanitization

**Markdown Content Security:**
- Exercise descriptions are stored as markdown (max 20000 characters)
- The application uses the `goldmark` markdown renderer (github.com/yuin/goldmark)
- **Note**: Goldmark does NOT sanitize HTML by default and allows raw HTML passthrough
- **Mitigation**: The application's Content Security Policy (CSP) provides defense-in-depth protection:
  - `script-src` requires cryptographic nonce - inline scripts without nonce are blocked
  - `object-src 'none'` prevents object/embed tag injection
  - `base-uri 'none'` prevents base tag hijacking attacks
  - `default-src 'none'` with explicit allowlists for each resource type
- **Accepted Risk**: While goldmark allows HTML passthrough, the strict CSP prevents XSS exploitation
- **Future Enhancement**: Consider adding explicit HTML sanitization with bluemonday or disabling raw HTML in goldmark for additional defense-in-depth

**Input Validation:**
- Exercise names: max 124 characters, must be unique per user
- Description markdown: max 20000 characters
- Category: enum validation (full_body, upper, lower)
- Exercise type: enum validation (weighted, bodyweight)
- Rate limiting should be implemented to prevent exercise creation abuse

### Required Security Tests

The following security test scenarios must be implemented:

1. **Cross-User Access Prevention:**
   - User A cannot GET user B's exercise by ID (should return 404)
   - User A cannot UPDATE user B's exercise (should return 404)
   - User A cannot DELETE user B's exercise (should return 404)
   - Unauthenticated user cannot see any user exercises (only system exercises)

2. **Context Authentication:**
   - Verify that user_id is extracted from authenticated context, not request parameters
   - Test that manipulating URL parameters cannot bypass ownership checks
   - Test that forged user_id in request body is ignored

3. **Data Isolation:**
   - List queries for User A do not return User B's exercises
   - Search/filter operations respect user boundaries
   - Exercise counts and statistics don't leak information about other users' exercises

4. **Cascading Deletion:**
   - Deleting a user cascades to delete their exercises
   - Deleting a user's exercise cascades to remove it from their workout history
   - Verify no orphaned records remain after cascading deletes

## Migration Strategy

1. **Schema Migration**: Add nullable user_id column to exercises table
2. **Data Migration**: Existing exercises get NULL user_id (system exercises)
3. **Code Migration**: Update repository layer first (backward compatible)
4. **Service Migration**: Update service layer to pass user context
5. **Handler Migration**: Update handlers to use new service methods
6. **Testing**: Add tests for user exercise visibility and ownership

## Open Questions

1. **Exercise Deletion**: Should users be able to delete their exercises if used in historical workouts?
   - **Recommendation**: Use CASCADE DELETE - deleting exercise removes it from all workouts (workout history preserved but exercise details lost)
   - **Alternative**: Soft delete with `deleted_at` timestamp to preserve history

2. **Exercise Limits**: Should there be a limit on how many exercises a user can create?
   - **Recommendation**: Start without limits, add if abuse occurs

3. **Exercise Sharing**: Should users be able to share exercises with other users?
   - **Recommendation**: Not in v1, could be added later with additional visibility flags

4. **Import/Export**: Should users be able to export/import exercise definitions?
   - **Recommendation**: Include in user data export for GDPR compliance, import can be added later

## Success Metrics

1. **Adoption**: Percentage of users who create at least one custom exercise
2. **Usage**: Number of custom exercises created per user
3. **Retention**: Whether custom exercise users have higher retention
4. **Performance**: Query performance remains within acceptable bounds (<100ms for exercise lists)

## Out of Scope for v1

- Exercise sharing between users
- Exercise templates or marketplace
- Bulk exercise import/export
- Exercise versioning or history
- Advanced muscle group visualization
- Exercise categorization beyond existing category field
