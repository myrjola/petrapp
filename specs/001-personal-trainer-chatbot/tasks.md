# Tasks: AI Personal Trainer Chatbot

**Input**: Design documents from `/specs/001-personal-trainer-chatbot/`
**Prerequisites**: plan.md (required), research.md, data-model.md, contracts/

## Execution Flow (main)
```
1. Load plan.md from feature directory
   → Tech stack: Go 1.21+, OpenAI SDK, SQLite, ECharts
   → Structure: Go web app with integrated chatbot
   → Libraries: internal/chatbot, internal/tools
2. Load design documents:
   → data-model.md: 3 entities (Conversation, ChatMessage, MessageVisualization)
   → contracts/: LLM function calling schema with 6 functions
   → research.md: OpenAI integration patterns, security boundaries
3. Generate tasks by category:
   → Setup: database migrations, chatbot service structure
   → Tests: contract tests for LLM functions, integration tests
   → Core: chatbot service, LLM integration, visualization generation
   → Integration: HTTP handlers, templates, WebSocket streaming
   → Polish: performance optimization, security validation
4. Apply task rules:
   → Different files = mark [P] for parallel
   → Tests before implementation (TDD)
   → Database first approach per PetrApp conventions
5. Tasks numbered T001-T035
6. Dependencies: Setup → Tests → Models → Services → Handlers → Polish
```

## Format: `[ID] [P?] Description`
- **[P]**: Can run in parallel (different files, no dependencies)
- Include exact file paths in descriptions

## Phase 3.1: Setup
- [ ] T001 Create database schema for conversations table in `internal/sqlite/schema.sql`
- [ ] T002 Create database schema for chat_messages table in `internal/sqlite/schema.sql`
- [ ] T003 Create database schema for message_visualizations table in `internal/sqlite/schema.sql`
- [ ] T004 Initialize chatbot service structure in `internal/chatbot/service.go`
- [ ] T005 [P] Configure OpenAI client setup in `internal/chatbot/llm_client.go`

## Phase 3.2: Tests First (TDD) ⚠️ MUST COMPLETE BEFORE 3.3
**CRITICAL: These tests MUST be written and MUST FAIL before ANY implementation**
- [ ] T006 [P] Contract test query_workout_data function in `internal/chatbot/contracts/query_test.go`
- [ ] T007 [P] Contract test generate_visualization function in `internal/chatbot/contracts/visualization_test.go`
- [ ] T008 [P] Contract test calculate_statistics function in `internal/chatbot/contracts/statistics_test.go`
- [ ] T009 [P] Contract test get_exercise_info function in `internal/chatbot/contracts/exercise_info_test.go`
- [ ] T010 [P] Contract test generate_workout_recommendation function in `internal/chatbot/contracts/recommendation_test.go`
- [ ] T011 [P] Contract test analyze_workout_pattern function in `internal/chatbot/contracts/pattern_test.go`
- [ ] T012 [P] Integration test conversation creation in `internal/chatbot/integration/conversation_test.go`
- [ ] T013 [P] Integration test message processing in `internal/chatbot/integration/message_test.go`
- [ ] T014 [P] Integration test user data isolation in `internal/chatbot/integration/isolation_test.go`
- [ ] T015 [P] Integration test visualization generation in `internal/chatbot/integration/visualization_test.go`

## Phase 3.3: Core Implementation (ONLY after tests are failing)
- [ ] T016 [P] Conversation model in `internal/chatbot/models/conversation.go`
- [ ] T017 [P] ChatMessage model in `internal/chatbot/models/message.go`
- [ ] T018 [P] MessageVisualization model in `internal/chatbot/models/visualization.go`
- [ ] T019 ConversationService CRUD operations in `internal/chatbot/service.go`
- [ ] T020 [P] LLM function tool: query_workout_data in `internal/chatbot/tools/query_tool.go`
- [ ] T021 [P] LLM function tool: generate_visualization in `internal/chatbot/tools/visualization_tool.go`
- [ ] T022 [P] LLM function tool: calculate_statistics in `internal/chatbot/tools/statistics_tool.go`
- [ ] T023 [P] LLM function tool: get_exercise_info in `internal/chatbot/tools/exercise_info_tool.go`
- [ ] T024 [P] LLM function tool: generate_workout_recommendation in `internal/chatbot/tools/recommendation_tool.go`
- [ ] T025 [P] LLM function tool: analyze_workout_pattern in `internal/chatbot/tools/pattern_tool.go`
- [ ] T026 OpenAI GPT-4 integration with function calling in `internal/chatbot/llm_client.go`

## Phase 3.4: Integration
- [ ] T027 GET /chat HTTP handler for conversation list in `cmd/web/chat_handlers.go`
- [ ] T028 GET /chat/{conversationID} HTTP handler for conversation view in `cmd/web/chat_handlers.go`
- [ ] T029 POST /chat/{conversationID}/message HTTP handler for message sending in `cmd/web/chat_handlers.go`
- [ ] T030 Chat UI template with conversation list in `ui/templates/pages/chat-list.gohtml`
- [ ] T031 Chat conversation template with message history in `ui/templates/pages/chat-conversation.gohtml`
- [ ] T032 ECharts visualization rendering in conversation template using existing pattern
- [ ] T033 Add chat routes to main router in `cmd/web/routes.go`
- [ ] T034 Add authentication middleware for chat endpoints in chat handlers

## Phase 3.5: Polish
- [ ] T035 [P] Performance test for 100 concurrent conversations in `test/performance/chatbot_test.go`
- [ ] T036 [P] Unit tests for message validation in `internal/chatbot/models/message_test.go`
- [ ] T037 [P] Unit tests for conversation state management in `internal/chatbot/models/conversation_test.go`
- [ ] T038 Security audit for user data isolation in all chatbot components
- [ ] T039 Token usage monitoring and rate limiting implementation
- [ ] T040 Error handling for OpenAI API failures and timeouts
- [ ] T041 Run quickstart.md validation scenarios

## Dependencies
- Database migrations (T001-T003) before models (T016-T018)
- Tests (T006-T015) before implementation (T016-T041)
- Models (T016-T018) before service (T019)
- Service (T019) before tools (T020-T025)
- Tools and LLM client (T020-T026) before handlers (T027-T029)
- Handlers before templates (T030-T032)
- Core implementation before polish (T035-T041)

## Parallel Example
```bash
# Launch database migrations together (T001-T003):
Task agent: "Create database schema for conversations table in `internal/sqlite/schema.sql`"
Task agent: "Create database schema for chat_messages table in `internal/sqlite/schema.sql`"
Task agent: "Create database schema for message_visualizations table in `internal/sqlite/schema.sql`"

# Launch contract tests together (T006-T011):
Task agent: "Contract test query_workout_data function in internal/chatbot/contracts/query_test.go"
Task agent: "Contract test generate_visualization function in internal/chatbot/contracts/visualization_test.go"
Task agent: "Contract test calculate_statistics function in internal/chatbot/contracts/statistics_test.go"

# Launch model creation together (T016-T018):
Task agent: "Conversation model in internal/chatbot/models/conversation.go"
Task agent: "ChatMessage model in internal/chatbot/models/message.go"
Task agent: "MessageVisualization model in internal/chatbot/models/visualization.go"

# Launch LLM function tools together (T020-T025):
Task agent: "LLM function tool: query_workout_data in internal/chatbot/tools/query_tool.go"
Task agent: "LLM function tool: generate_visualization in internal/chatbot/tools/visualization_tool.go"
Task agent: "LLM function tool: calculate_statistics in internal/chatbot/tools/statistics_tool.go"
```

## Notes
- Follow PetrApp's "Database First" architectural flow
- Use existing patterns: POST-Redirect-GET, SecureQueryTool, ECharts integration
- All SQL queries must be parameterized and use user isolation
- Follow Go conventions: 100-line function limit, error suffixes, no global loggers
- Use structured logging with slog throughout
- Leverage existing userdb package for GDPR-compliant user isolation
- [P] tasks = different files, no dependencies
- Verify tests fail before implementing
- Run `make ci` after completing core implementation
- Commit after each task completion

## Task Generation Rules
*Applied during execution*

1. **From Data Model (3 entities)**:
   - Each entity → model creation task [P] (T016-T018)
   - Relationships → service layer tasks (T019)

2. **From Contracts (6 LLM functions)**:
   - Each function → contract test task [P] (T006-T011)
   - Each function → implementation task [P] (T020-T025)

3. **From Quickstart Scenarios**:
   - Each test scenario → integration test [P] (T012-T015)
   - Validation scenarios → polish task (T041)

4. **From Architecture (Go web app)**:
   - Database migrations first (T001-T003)
   - HTTP handlers for chat interface (T027-T029)
   - Go HTML templates following existing patterns (T030-T032)
   - Route integration (T033-T034)

## Validation Checklist
- [x] All 6 LLM functions have contract tests (T006-T011)
- [x] All 3 entities have model tasks (T016-T018)
- [x] All tests come before implementation (T006-T015 before T016-T041)
- [x] Parallel tasks truly independent (different files)
- [x] Each task specifies exact file path
- [x] Database-first approach follows PetrApp conventions
- [x] Security boundaries maintained (user isolation, SecureQueryTool)
- [x] Performance requirements addressed (T035, T039)
- [x] TDD ordering enforced (tests must fail before implementation)