# WebSocket Protocol Specification

## Connection

### Endpoint
`wss://[domain]/api/v1/chat/ws`

### Authentication
- Session cookie must be present
- Connection rejected if not authenticated

## Message Format

All messages use JSON format with the following structure:

```json
{
  "type": "message_type",
  "payload": { ... }
}
```

## Client → Server Messages

### 1. Start Conversation
```json
{
  "type": "start_conversation",
  "payload": {
    "title": "Optional conversation title"
  }
}
```

### 2. Send Message
```json
{
  "type": "send_message",
  "payload": {
    "conversation_id": 123,
    "content": "User's message text"
  }
}
```

### 3. Close Conversation
```json
{
  "type": "close_conversation",
  "payload": {
    "conversation_id": 123
  }
}
```

## Server → Client Messages

### 1. Conversation Started
```json
{
  "type": "conversation_started",
  "payload": {
    "conversation_id": 123,
    "title": "Conversation title"
  }
}
```

### 2. Message Received
```json
{
  "type": "message_received",
  "payload": {
    "message_id": 456,
    "conversation_id": 123,
    "timestamp": "2025-09-14T10:30:00Z"
  }
}
```

### 3. Response Stream Start
```json
{
  "type": "stream_start",
  "payload": {
    "message_id": 457,
    "conversation_id": 123
  }
}
```

### 4. Response Stream Chunk
```json
{
  "type": "stream_chunk",
  "payload": {
    "message_id": 457,
    "content": "Partial response text..."
  }
}
```

### 5. Response Stream End
```json
{
  "type": "stream_end",
  "payload": {
    "message_id": 457,
    "total_tokens": 150,
    "execution_time_ms": 2500
  }
}
```

### 6. Visualization Generated
```json
{
  "type": "visualization",
  "payload": {
    "message_id": 457,
    "visualization_id": 789,
    "chart_type": "line",
    "chart_config": { /* ECharts config */ }
  }
}
```

### 7. Error
```json
{
  "type": "error",
  "payload": {
    "code": "QUERY_TIMEOUT",
    "message": "Query execution timed out",
    "details": { ... }
  }
}
```

## Error Codes

| Code | Description |
|------|-------------|
| `AUTH_FAILED` | Authentication failed |
| `CONVERSATION_NOT_FOUND` | Conversation doesn't exist or not owned by user |
| `RATE_LIMITED` | Too many messages |
| `QUERY_TIMEOUT` | Database query timed out |
| `QUERY_ERROR` | Database query failed |
| `LLM_ERROR` | OpenAI API error |
| `INVALID_MESSAGE` | Message format invalid |

## Connection Management

### Heartbeat
- Server sends ping every 30 seconds
- Client must respond with pong
- Connection closed after 3 missed pongs

### Reconnection
- Client should implement exponential backoff
- Start with 1 second, max 30 seconds
- Include last message ID for continuity

## Rate Limiting

- Max 10 messages per minute per user
- Max 100 messages per hour per user
- Limits returned in error response