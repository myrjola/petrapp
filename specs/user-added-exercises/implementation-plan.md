# User Added Exercises - Implementation Plan

## Overview

This document provides a step-by-step implementation plan for adding user-specific exercises to PetrApp. The implementation follows the shared table approach recommended in the specification.

## Implementation Phases

### Phase 1: Database Schema (Foundation)

**Objective**: Update database schema to support user-owned exercises.

**Tasks**:
1. Update `schema.sql` to add `user_id` column to exercises table
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
   ```

2. Update the exercises table definition in `schema.sql`
   - Change `name TEXT NOT NULL UNIQUE` to `name TEXT NOT NULL`
   - Add `UNIQUE (name, user_id)` constraint after the column definitions
   - This allows system exercises (user_id = NULL) and user exercises to have duplicate names across users

3. Update `fixtures.sql` to explicitly set `user_id = NULL` for system exercises
   - While NULL is default, being explicit in fixtures improves clarity
   - Helps document the system exercise convention

4. Test schema migration
   - Run `make test` to ensure migration system accepts changes
   - Verify that existing exercises remain unaffected (user_id = NULL)
   - Test that the composite unique constraint allows duplicate names for different users

**Files to Modify**:
- `internal/sqlite/schema.sql`
- `internal/sqlite/fixtures.sql`

**Testing**:
- Existing tests should pass (backward compatible change)
- Database migration should complete successfully

---

### Phase 2: Repository Layer (Data Access)

**Objective**: Update exercise repository to filter by user ownership.

**Tasks**:

1. **Add user context helper** (`internal/workout/repository.go`)
   - Create helper function to extract user_id from context
   - Use existing `contexthelpers.AuthenticatedUserID(ctx)` pattern
   ```go
   func getUserIDFromContext(ctx context.Context) int {
       return contexthelpers.AuthenticatedUserID(ctx)
   }
   ```
   - **Important**: This returns 0 for unauthenticated requests
   - Since SQLite INTEGER PRIMARY KEY starts at 1, user_id = 0 safely restricts to system exercises
   - Queries like `WHERE user_id IS NULL OR user_id = ?` with value 0 only return system exercises

2. **Update `List()` method** (`internal/workout/repository-exercises.go`)
   - Modify query to return system exercises + user's exercises
   ```sql
   SELECT id, name, category, exercise_type, description_markdown
   FROM exercises
   WHERE user_id IS NULL OR user_id = ?
   ORDER BY id
   ```
   - Extract user_id from context
   - Handle unauthenticated context (return only system exercises)

3. **Update `Get()` method** (`internal/workout/repository-exercises.go`)
   - Add visibility check after fetching exercise
   - Return `ErrNotFound` if exercise belongs to different user
   ```go
   if exercise.UserID != nil && *exercise.UserID != userID {
       return Exercise{}, ErrNotFound
   }
   ```
   - **Security Pattern**: Return `ErrNotFound` for authorization failures (not a distinct "forbidden" error)
   - This prevents user enumeration attacks - attackers cannot distinguish between:
     - "Exercise doesn't exist" vs "Exercise exists but you can't access it"
   - This is an intentional security design decision to avoid information leakage

4. **Update `Create()` method** (`internal/workout/repository-exercises.go`)
   - Extract user_id from context
   - Include user_id in INSERT statement
   ```sql
   INSERT INTO exercises (name, category, exercise_type, description_markdown, user_id)
   VALUES (?, ?, ?, ?, ?)
   ```
   - Use NULL for system exercises (admin-only feature, future work)

5. **Update `Update()` method** (`internal/workout/repository-exercises.go`)
   - Add ownership check before allowing update
   - Return error if user doesn't own exercise
   ```go
   if exercise.UserID != nil && *exercise.UserID != userID {
       return fmt.Errorf("cannot update exercise owned by different user")
   }
   ```

**Files to Modify**:
- `internal/workout/repository-exercises.go`

**Domain Model Update**:
- Add `UserID *int` field to `Exercise` struct in `internal/workout/models.go`
- Update JSON schema in `exerciseJSONSchema` if needed

**Testing**:
- Add unit tests for repository methods with user context
- Test visibility rules (user can't see other user's exercises)
- Test ownership rules (user can't edit other user's exercises)

---

### Phase 3: Service Layer (Business Logic)

**Objective**: Update service methods to pass user context to repository.

**Tasks**:

1. **Update `List()` method** (`internal/workout/service.go`)
   - Service already calls `repo.exercises.List(ctx)`
   - Should work automatically with updated repository
   - No changes needed if repository handles context internally

2. **Update `GetExercise()` method** (`internal/workout/service.go`)
   - Repository already handles visibility
   - No changes needed

3. **Update `GenerateExercise()` method** (`internal/workout/service.go`)
   - Ensure created exercise gets user_id from context
   - Repository Create() should handle this automatically
   - No explicit changes needed

4. **Update `UpdateExercise()` method** (`internal/workout/service.go`)
   - Repository Update() already checks ownership
   - No changes needed

5. **Add exercise ownership validation helpers** (optional)
   - Consider adding service-level helpers for ownership checks
   - Could be useful for future admin features

**Files to Modify**:
- `internal/workout/service.go` (minimal changes, mostly verification)

**Testing**:
- Service tests should pass with updated repository
- Add integration tests for end-to-end exercise creation flow

---

### Phase 4: HTTP Handlers (User Interface)

**Objective**: Update web handlers to work with user-specific exercises.

**Tasks**:

1. **Review exercise listing handlers**
   - Identify all places where exercises are listed/selected
   - Examples: workout page, exercise swap, add exercise
   - Verify they use `workoutService.List(ctx)` which now filters correctly

2. **Add visual indicators for user exercises**
   - Update templates to show badge/icon for user-created exercises
   - Check `Exercise.UserID` in template to determine if user-created
   - Example: Show "Custom" badge or user icon next to exercise name

3. **Update exercise info handler** (`cmd/web/handler-exercise-info.go`)
   - Add "Edit" button only if user owns exercise (UserID matches authenticated user)
   - Could add `Editable bool` field to template data for clarity

4. **Create exercise creation UI** (if not exists)
   - May already exist via GenerateExercise flow
   - Ensure it's accessible to all users (not just admins)
   - Wire up to existing `GenerateExercise` service method

5. **Add exercise management page** (optional for v1)
   - List user's custom exercises
   - Allow editing/deleting
   - Could be added later

**Files to Modify**:
- `cmd/web/handler-exerciseset.go` (add editability checks)
- `cmd/web/handler-exercise-info.go` (add edit controls)
- Templates in `ui/templates/pages/` (add visual indicators)

**Testing**:
- End-to-end tests for exercise visibility in UI
- Test that users can't access edit pages for exercises they don't own
- Test visual indicators appear correctly

---

### Phase 5: Templates & UI (Frontend)

**Objective**: Update templates to show user exercise ownership and enable editing.

**Tasks**:

1. **Update exercise list templates**
   - Add visual indicator for user exercises
   - Example in template:
   ```html
   {{if .Exercise.UserID}}
     <span class="badge">Custom</span>
   {{end}}
   ```

2. **Update exercise detail template**
   - Show "Edit" button only for user-owned exercises
   - Use scoped CSS for any new styling (per template guidelines)

3. **Update exercise selector templates**
   - Ensure user exercises appear in selection lists
   - Maintain visual distinction between system and user exercises

4. **Add accessibility considerations**
   - Ensure screen readers announce custom exercise status
   - Use semantic HTML and ARIA labels where appropriate

**Files to Modify**:
- `ui/templates/pages/exerciseset/` (exercise detail view)
- `ui/templates/pages/workout/` (exercise selection)
- Templates for any other exercise listing views

**Testing**:
- Manual testing of UI with multiple users
- Verify visual indicators appear correctly
- Test responsive design

---

### Phase 6: Testing & Validation

**Objective**: Comprehensive testing of user exercise features.

**Tasks**:

1. **Unit Tests**
   - Repository layer tests for ownership filtering
   - Service layer tests for user context propagation
   - Test edge cases: unauthenticated users, NULL user_id

2. **Integration Tests**
   - End-to-end exercise creation flow
   - Exercise visibility across different users
   - Workout usage with user exercises

3. **Security Tests**
   - **Cross-User Access Prevention:**
     - User A cannot GET user B's exercise by ID (should return 404/ErrNotFound)
     - User A cannot UPDATE user B's exercise (should return 404/ErrNotFound)
     - User A cannot DELETE user B's exercise (should return 404/ErrNotFound)
     - Unauthenticated user cannot see any user exercises (only system exercises)
   - **Context Authentication:**
     - Verify that user_id is extracted from authenticated context, not request parameters
     - Test that manipulating URL parameters cannot bypass ownership checks
     - Test that forged user_id in request body is ignored
   - **Data Isolation:**
     - List queries for User A do not return User B's exercises
     - Search/filter operations respect user boundaries
     - Exercise counts and statistics don't leak information about other users' exercises
   - **Cascading Deletion:**
     - Deleting a user cascades to delete their exercises
     - Deleting a user's exercise cascades to remove it from their workout history
     - Verify no orphaned records remain after cascading deletes

4. **Performance Tests**
   - Measure query performance with user filtering
   - Ensure <100ms for exercise list queries
   - Check for N+1 queries

5. **Manual Testing**
   - Create exercises as different users
   - Verify visibility rules
   - Test workout integration
   - Test data export includes user exercises

**Testing Checklist**:
- [ ] Repository tests pass
- [ ] Service tests pass
- [ ] Handler tests pass
- [ ] End-to-end tests pass
- [ ] Security tests pass
- [ ] Performance is acceptable
- [ ] Manual testing complete

---

### Phase 7: Data Export (GDPR Compliance)

**Objective**: Ensure user data export includes custom exercises.

**Tasks**:

1. **Review existing data export** (`internal/workout/service.go:ExportUserData`)
   - Check if exercises are already included in export
   - If exercises table is already exported, user exercises will be included automatically

2. **Verify export includes user_id column**
   - Ensure exported schema includes user_id
   - User should be able to identify their custom exercises in export

3. **Test export functionality**
   - Create user with custom exercises
   - Export data
   - Verify exercises are included with correct user_id

**Files to Review**:
- `internal/workout/service.go` (ExportUserData method)
- `internal/sqlite/database.go` (CreateUserDB method)

---

## Implementation Order

**Recommended sequence** (each phase depends on previous):

1. Phase 1: Database Schema ✓ (foundation)
2. Phase 2: Repository Layer ✓ (data access)
3. Phase 3: Service Layer ✓ (business logic)
4. Phase 4: HTTP Handlers ✓ (API)
5. Phase 5: Templates & UI ✓ (frontend)
6. Phase 6: Testing ✓ (validation)
7. Phase 7: Data Export ✓ (compliance)

Each phase should be completed and tested before moving to the next.

---

## Rollout Strategy

### Development
1. Implement behind feature flag (optional)
2. Test with development database
3. Create sample user exercises for testing

### Staging
1. Deploy schema changes first
2. Deploy code changes
3. Verify migration succeeds
4. Test with multiple test users

### Production
1. Deploy schema changes (backward compatible)
2. Deploy code changes
3. Monitor error rates and performance
4. Gather user feedback

---

## Rollback Plan

If issues arise after deployment:

1. **Schema rollback**: Drop user_id column
   ```sql
   DROP INDEX exercises_user_id_idx;
   ALTER TABLE exercises DROP COLUMN user_id;
   ```
   - Safe if no users have created exercises yet
   - Data loss if users have created exercises

2. **Code rollback**: Revert to previous version
   - Repository queries revert to unfiltered
   - User exercises become system exercises (visible to all)

3. **Mitigation**: Use feature flag
   - If implemented with feature flag, can disable feature without rollback
   - Recommended for production deployment

---

## Monitoring & Metrics

After deployment, monitor:

1. **Query Performance**
   - Exercise list query time
   - Watch for slow queries with user_id filtering

2. **Usage Metrics**
   - Number of custom exercises created
   - Percentage of users creating exercises
   - Most common exercise types created

3. **Error Rates**
   - 404s for exercise not found
   - Permission errors for exercise access
   - Database errors

4. **User Feedback**
   - Support requests related to custom exercises
   - Bug reports
   - Feature requests

---

## Future Enhancements

Once v1 is stable, consider:

1. **Exercise Sharing**: Allow users to share exercises with specific users or make public
2. **Exercise Templates**: Pre-made templates users can customize
3. **Bulk Import**: Import exercises from CSV or other formats
4. **Exercise Analytics**: Show which user exercises are most popular
5. **Exercise Versioning**: Track changes to exercise descriptions over time
6. **Admin Controls**: Admin panel for managing system exercises

---

## Resources & References

- **Architecture Guidelines**: `/home/runner/work/petrapp/petrapp/CLAUDE.md`
- **Database Guidelines**: `/home/runner/work/petrapp/petrapp/internal/sqlite/CLAUDE.md`
- **Domain Guidelines**: `/home/runner/work/petrapp/petrapp/internal/workout/CLAUDE.md`
- **Web Guidelines**: `/home/runner/work/petrapp/petrapp/cmd/web/CLAUDE.md`
- **Template Guidelines**: `/home/runner/work/petrapp/petrapp/ui/templates/CLAUDE.md`

---

## Estimated Effort

| Phase | Estimated Time | Complexity |
|-------|---------------|------------|
| 1. Database Schema | 1 hour | Low |
| 2. Repository Layer | 3-4 hours | Medium |
| 3. Service Layer | 1-2 hours | Low |
| 4. HTTP Handlers | 2-3 hours | Medium |
| 5. Templates & UI | 2-3 hours | Medium |
| 6. Testing | 4-5 hours | Medium |
| 7. Data Export | 1 hour | Low |
| **Total** | **14-19 hours** | **Medium** |

**Note**: Estimates assume familiarity with codebase and no major blockers.

---

## Success Criteria

The implementation is complete when:

1. ✓ Schema migration adds user_id column successfully
2. ✓ Users can create custom exercises
3. ✓ Custom exercises only visible to creator
4. ✓ Custom exercises work in workouts (add, swap, track)
5. ✓ Users can edit their own exercises
6. ✓ System exercises remain uneditable by regular users
7. ✓ All tests pass
8. ✓ Performance meets requirements (<100ms queries)
9. ✓ Data export includes custom exercises
10. ✓ No security vulnerabilities (users can't access others' exercises)

---

## Getting Started

To begin implementation:

1. Review this plan and the specification document
2. Set up development environment with `make init`
3. Create a feature branch: `git checkout -b feature/user-exercises`
4. Start with Phase 1 (Database Schema)
5. Run tests after each phase: `make test`
6. Commit frequently with descriptive messages
7. Use `make ci` for comprehensive validation before PR

For questions or clarification, refer to the specification document or consult the team.
