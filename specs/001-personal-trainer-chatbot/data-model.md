# Data Model: AI Personal Trainer Chatbot

**Date**: 2025-09-14
**Feature**: AI Personal Trainer Chatbot
**Branch**: `001-personal-trainer-chatbot`

## Entity Relationship Diagram

```
User (existing)
  |
  1:N
  |
Conversation
  |
  1:N
  |
ChatMessage
  |
  1:N
  |
MessageVisualization
```

## Entity Definitions

### 1. Conversation

Represents an interaction session between a user and the AI trainer.

**Fields**:
- `id` (INTEGER PRIMARY KEY): Unique identifier
- `user_id` (INTEGER NOT NULL): Reference to user, foreign key
- `title` (TEXT): Auto-generated conversation title
- `created_at` (TIMESTAMP NOT NULL): When conversation started
- `updated_at` (TIMESTAMP NOT NULL): Last activity in conversation
- `is_active` (BOOLEAN DEFAULT true): Whether conversation is active
- `context_summary` (TEXT): LLM-generated summary for context maintenance

**Validation Rules**:
- `user_id` must exist in users table
- `created_at` <= `updated_at`
- `title` max length 200 characters

**State Transitions**:
- Created → Active (on first message)
- Active → Inactive (after 30 minutes of inactivity or explicit close)
- Inactive → Active (when user resumes)

### 2. ChatMessage

Individual message in a conversation.

**Fields**:
- `id` (INTEGER PRIMARY KEY): Unique identifier
- `conversation_id` (INTEGER NOT NULL): Reference to conversation
- `message_type` (TEXT NOT NULL): 'user' or 'assistant'
- `content` (TEXT NOT NULL): Message text content
- `created_at` (TIMESTAMP NOT NULL): When message was created
- `token_count` (INTEGER): OpenAI token usage for this message
- `error_message` (TEXT): Error details if query failed
- `query_executed` (TEXT): SQL query that was executed (if any)
- `execution_time_ms` (INTEGER): Query execution time in milliseconds

**Validation Rules**:
- `conversation_id` must exist in conversations table
- `message_type` IN ('user', 'assistant')
- `content` required, max 10000 characters
- `token_count` >= 0 when present
- `execution_time_ms` >= 0 when present

**Indexes**:
- `conversation_id, created_at` for message ordering
- `message_type` for filtering

### 3. MessageVisualization

Charts/graphs generated from workout data in response to queries.

**Fields**:
- `id` (INTEGER PRIMARY KEY): Unique identifier
- `message_id` (INTEGER NOT NULL): Reference to chat message
- `chart_type` (TEXT NOT NULL): Type of chart (line, bar, scatter, etc.)
- `chart_config` (JSON NOT NULL): ECharts configuration object
- `data_query` (TEXT NOT NULL): SQL query used to generate data
- `created_at` (TIMESTAMP NOT NULL): When visualization was created

**Validation Rules**:
- `message_id` must exist in chat_messages table
- `chart_type` IN ('line', 'bar', 'scatter', 'pie', 'heatmap')
- `chart_config` must be valid JSON
- `data_query` must be non-empty

## Relationships

### User → Conversation (1:N)
- One user can have multiple conversations
- Conversations are isolated per user
- Cascade delete when user is deleted

### Conversation → ChatMessage (1:N)
- One conversation contains multiple messages
- Messages are ordered by created_at
- Cascade delete when conversation is deleted

### ChatMessage → MessageVisualization (1:N)
- One message can generate multiple visualizations
- Visualizations reference the message that triggered them
- Cascade delete when message is deleted

## Query Patterns

### Get User's Conversations
```sql
SELECT id, title, updated_at, is_active
FROM conversations
WHERE user_id = ?
ORDER BY updated_at DESC
LIMIT 20
```

### Get Conversation Messages
```sql
SELECT id, message_type, content, created_at, error_message
FROM chat_messages
WHERE conversation_id = ?
ORDER BY created_at ASC
```

### Get Message Visualizations
```sql
SELECT id, chart_type, chart_config
FROM message_visualizations
WHERE message_id = ?
ORDER BY created_at ASC
```

### Update Conversation Activity
```sql
UPDATE conversations
SET updated_at = CURRENT_TIMESTAMP,
    is_active = true
WHERE id = ? AND user_id = ?
```

## Migration Strategy

```sql
-- Create conversations table
CREATE TABLE conversations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    title TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    is_active BOOLEAN DEFAULT true,
    context_summary TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_conversations_user_id ON conversations(user_id);
CREATE INDEX idx_conversations_updated_at ON conversations(updated_at);

-- Create chat_messages table
CREATE TABLE chat_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id INTEGER NOT NULL,
    message_type TEXT NOT NULL CHECK(message_type IN ('user', 'assistant')),
    content TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    token_count INTEGER,
    error_message TEXT,
    query_executed TEXT,
    execution_time_ms INTEGER,
    FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
);

CREATE INDEX idx_chat_messages_conversation ON chat_messages(conversation_id, created_at);
CREATE INDEX idx_chat_messages_type ON chat_messages(message_type);

-- Create message_visualizations table
CREATE TABLE message_visualizations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    message_id INTEGER NOT NULL,
    chart_type TEXT NOT NULL CHECK(chart_type IN ('line', 'bar', 'scatter', 'pie', 'heatmap')),
    chart_config JSON NOT NULL,
    data_query TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (message_id) REFERENCES chat_messages(id) ON DELETE CASCADE
);

CREATE INDEX idx_message_visualizations_message ON message_visualizations(message_id);
```

## Domain Model Integration

The chatbot data model integrates with existing workout domain models:

### Reading Workout Data
- Queries will access existing tables: `workout_sessions`, `exercise_sets`, `exercises`
- User isolation maintained through `user_id` foreign keys
- Read-only access via SecureQueryTool

### Generating Insights
- Aggregate data from `exercise_sets` for progression analysis
- Use `muscle_groups` and `exercise_muscle_groups` for workout balance
- Reference `workout_preferences` for personalized recommendations

## Privacy & Security Considerations

1. **User Isolation**: All queries filtered by user_id
2. **Conversation Privacy**: Conversations never shared between users
3. **Query Audit**: All executed queries logged in chat_messages
4. **Token Limits**: Track token usage to prevent abuse
5. **Data Retention**: Follow GDPR requirements for conversation history

## Performance Considerations

1. **Indexing**: Strategic indexes on foreign keys and timestamp columns
2. **Pagination**: Limit conversation and message queries
3. **Caching**: Consider caching frequent visualization queries
4. **Cleanup**: Periodic cleanup of old inactive conversations