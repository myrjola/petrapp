# Research Report: AI Personal Trainer Chatbot

**Date**: 2025-09-14
**Feature**: AI Personal Trainer Chatbot
**Branch**: `001-personal-trainer-chatbot`

## Executive Summary

The PetrApp codebase already contains substantial enabling functionality for implementing an AI personal trainer chatbot. Key components like user database isolation, secure query execution, OpenAI integration, and ECharts visualization are already in place. The implementation can leverage these existing patterns while adding chatbot-specific logic.

## Technology Decisions

### 1. LLM Integration Approach
**Decision**: Use OpenAI GPT-4 with function calling via existing SDK
**Rationale**:
- OpenAI SDK already integrated (`github.com/openai/openai-go v1.12.0`)
- API key configuration exists in environment
- Function calling provides structured interaction with database queries
**Alternatives considered**:
- Claude API - would require new SDK integration
- Local LLMs - insufficient for complex query understanding

### 2. Database Query Strategy
**Decision**: Use existing `tools.SecureQueryTool` with user-isolated databases
**Rationale**:
- SecureQueryTool already implements security boundaries (read-only, timeouts, row limits)
- userdb package provides GDPR-compliant user isolation
- Automatic foreign key discovery ensures complete data isolation
**Alternatives considered**:
- Direct SQL access - security risk
- ORM-based queries - less flexible for natural language conversion

### 3. Conversation Storage
**Decision**: SQLite tables for conversation history (permanent storage)
**Rationale**:
- Consistent with existing SQLite-based architecture
- Meets FR-011 requirement for permanent history
- Enables conversation context and follow-ups
**Alternatives considered**:
- In-memory storage - doesn't meet persistence requirement
- Redis - adds unnecessary dependency

### 4. Visualization Generation
**Decision**: Server-side data preparation with client-side ECharts rendering
**Rationale**:
- Existing ECharts integration pattern in exercise-info.gohtml
- Dynamic module loading already implemented
- Interactive charts with tooltips working
**Alternatives considered**:
- Server-side chart generation - less interactive
- D3.js - would require new library integration

### 5. User Interface Integration
**Decision**: Chat widget embedded in existing pages with WebSocket for real-time
**Rationale**:
- Maintains consistency with existing UI
- WebSocket enables streaming responses
- Progressive enhancement approach
**Alternatives considered**:
- Separate chat page - disrupts user workflow
- Polling-based updates - higher latency

## Architecture Patterns Discovered

### Service Layer Pattern
```go
type ChatbotService struct {
    db           *sql.DB
    openaiClient *openai.Client
    queryTool    *tools.SecureQueryTool
    userDB       *sqlite.UserDB
}
```

### Handler Pattern
```go
func (app *application) chatbotHandler(w http.ResponseWriter, r *http.Request) {
    userID := contexthelpers.UserID(r.Context())
    // Implementation follows existing patterns
}
```

### Security Boundaries
1. User authentication via existing session management
2. User data isolation via userdb package
3. Query safety via SecureQueryTool
4. CSP nonce for inline scripts

## Implementation Considerations

### Performance
- Query timeout: 5 seconds (configurable)
- Response time target: <10 seconds total
- Row limit: 1000 rows per query
- Concurrent conversations: 100 (connection pooling required)

### Security
- PRAGMA QUERY_ONLY enforcement
- No ATTACH DATABASE allowed
- User ID validation on every request
- Conversation history scoped to user

### Observability
- Structured logging with slog
- Query execution metrics
- OpenAI API usage tracking
- Error rates by query type

## Resolved Clarifications

From the original specification, the following items are now resolved:

1. **Response time**: Use 10 seconds as per spec, implemented via context timeouts
2. **Concurrent users**: 100 simultaneous conversations via connection pooling
3. **Conversation retention**: Permanent storage in SQLite
4. **Recommendation criteria**: Based on:
   - Historical workout patterns
   - Progressive overload principles
   - Muscle group rotation
   - Rest day requirements

## Next Steps

1. Design conversation and message data models
2. Create OpenAPI contracts for chat endpoints
3. Define LLM function schemas for tool calling
4. Plan WebSocket protocol for streaming
5. Design conversation UI component

## Risk Mitigation

**Risk**: OpenAI API costs
**Mitigation**: Implement token limits and rate limiting per user

**Risk**: Malicious query injection via LLM
**Mitigation**: SecureQueryTool already filters dangerous operations

**Risk**: Data leakage between users
**Mitigation**: userdb isolation and context-based filtering

**Risk**: Long response times
**Mitigation**: Streaming responses, query optimization, caching

## Conclusion

The existing PetrApp architecture provides excellent foundations for the AI chatbot feature. The implementation can follow established patterns while adding chatbot-specific components. All technical clarifications from the specification have been resolved through research and existing codebase analysis.