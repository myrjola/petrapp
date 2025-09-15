package chatbot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// TokenUsageMonitor tracks and limits token usage per user.
type TokenUsageMonitor struct {
	db             *sqlite.Database
	logger         *slog.Logger
	dailyLimits    map[string]int // limits by user tier
	monthlyLimits  map[string]int
	usageCache     map[int]*UserUsage // userID -> usage
	cacheMutex     sync.RWMutex
	cacheExpiry    time.Duration
	lastCacheClean time.Time
}

// UserUsage tracks token consumption for a user.
type UserUsage struct {
	UserID          int
	DailyTokens     int
	MonthlyTokens   int
	LastUpdated     time.Time
	RequestCount    int
	LastRequestTime time.Time
	Tier            string
}

// TokenUsageEntry represents a token usage database record.
type TokenUsageEntry struct {
	ID             int       `json:"id"`
	UserID         int       `json:"user_id"`
	ConversationID int       `json:"conversation_id"`
	MessageID      int       `json:"message_id"`
	TokensUsed     int       `json:"tokens_used"`
	RequestType    string    `json:"request_type"` // "chat", "visualization", "analysis"
	Model          string    `json:"model"`        // "gpt-4", "gpt-3.5-turbo"
	Timestamp      time.Time `json:"timestamp"`
	ResponseTimeMs int       `json:"response_time_ms"`
}

// RateLimitError indicates rate limit exceeded.
type RateLimitError struct {
	UserID    int
	LimitType string // "daily", "monthly", "rate"
	Limit     int
	Current   int
	ResetTime time.Time
	Message   string
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded: %s", e.Message)
}

// NewTokenUsageMonitor creates a new token usage monitor.
func NewTokenUsageMonitor(db *sqlite.Database, logger *slog.Logger) *TokenUsageMonitor {
	return &TokenUsageMonitor{
		db:     db,
		logger: logger,
		dailyLimits: map[string]int{
			"free":    10000,   // 10k tokens per day for free users
			"premium": 100000,  // 100k tokens per day for premium users
			"admin":   1000000, // 1M tokens per day for admins
		},
		monthlyLimits: map[string]int{
			"free":    100000,   // 100k tokens per month for free users
			"premium": 2000000,  // 2M tokens per month for premium users
			"admin":   10000000, // 10M tokens per month for admins
		},
		usageCache:     make(map[int]*UserUsage),
		cacheExpiry:    5 * time.Minute,
		lastCacheClean: time.Now(),
	}
}

// CheckRateLimit validates if user can make a request given their usage.
func (tm *TokenUsageMonitor) CheckRateLimit(ctx context.Context, estimatedTokens int) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return errors.New("user not authenticated")
	}

	usage, err := tm.getUserUsage(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user usage: %w", err)
	}

	// Check rate limiting (requests per minute)
	now := time.Now()
	if now.Sub(usage.LastRequestTime) < time.Minute {
		usage.RequestCount++
	} else {
		usage.RequestCount = 1
		usage.LastRequestTime = now
	}

	maxRequestsPerMinute := tm.getRequestRateLimit(usage.Tier)
	if usage.RequestCount > maxRequestsPerMinute {
		return &RateLimitError{
			UserID:    userID,
			LimitType: "rate",
			Limit:     maxRequestsPerMinute,
			Current:   usage.RequestCount,
			ResetTime: usage.LastRequestTime.Add(time.Minute),
			Message:   fmt.Sprintf("too many requests: %d/%d per minute", usage.RequestCount, maxRequestsPerMinute),
		}
	}

	// Check daily token limit
	dailyLimit := tm.dailyLimits[usage.Tier]
	if usage.DailyTokens+estimatedTokens > dailyLimit {
		return &RateLimitError{
			UserID:    userID,
			LimitType: "daily",
			Limit:     dailyLimit,
			Current:   usage.DailyTokens,
			ResetTime: tm.getNextDayReset(),
			Message:   fmt.Sprintf("daily token limit exceeded: %d/%d tokens", usage.DailyTokens, dailyLimit),
		}
	}

	// Check monthly token limit
	monthlyLimit := tm.monthlyLimits[usage.Tier]
	if usage.MonthlyTokens+estimatedTokens > monthlyLimit {
		return &RateLimitError{
			UserID:    userID,
			LimitType: "monthly",
			Limit:     monthlyLimit,
			Current:   usage.MonthlyTokens,
			ResetTime: tm.getNextMonthReset(),
			Message:   fmt.Sprintf("monthly token limit exceeded: %d/%d tokens", usage.MonthlyTokens, monthlyLimit),
		}
	}

	return nil
}

// RecordUsage records token usage for a user.
func (tm *TokenUsageMonitor) RecordUsage(ctx context.Context, entry TokenUsageEntry) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return errors.New("user not authenticated")
	}

	entry.UserID = userID
	entry.Timestamp = time.Now()

	// Insert into database
	_, err := tm.db.ReadWrite.ExecContext(ctx, `
		INSERT INTO token_usage (
			user_id, conversation_id, message_id, tokens_used,
			request_type, model, timestamp, response_time_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.UserID, entry.ConversationID, entry.MessageID, entry.TokensUsed,
		entry.RequestType, entry.Model, entry.Timestamp.Format(time.RFC3339), entry.ResponseTimeMs)

	if err != nil {
		tm.logger.Error("Failed to record token usage",
			"user_id", userID,
			"tokens", entry.TokensUsed,
			"error", err)
		return fmt.Errorf("failed to record usage: %w", err)
	}

	// Update cache
	tm.cacheMutex.Lock()
	if usage, exists := tm.usageCache[userID]; exists {
		usage.DailyTokens += entry.TokensUsed
		usage.MonthlyTokens += entry.TokensUsed
		usage.LastUpdated = time.Now()
	}
	tm.cacheMutex.Unlock()

	tm.logger.Info("Recorded token usage",
		"user_id", userID,
		"tokens", entry.TokensUsed,
		"request_type", entry.RequestType,
		"model", entry.Model,
		"response_time_ms", entry.ResponseTimeMs)

	return nil
}

// GetUserUsage returns current usage statistics for a user.
func (tm *TokenUsageMonitor) GetUserUsage(ctx context.Context) (*UserUsage, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return nil, errors.New("user not authenticated")
	}

	return tm.getUserUsage(ctx, userID)
}

// GetUsageReport generates usage report for analytics.
func (tm *TokenUsageMonitor) GetUsageReport(ctx context.Context, startDate, endDate time.Time) ([]TokenUsageEntry, error) {
	rows, err := tm.db.ReadOnly.QueryContext(ctx, `
		SELECT id, user_id, conversation_id, message_id, tokens_used,
			   request_type, model, timestamp, response_time_ms
		FROM token_usage
		WHERE timestamp BETWEEN ? AND ?
		ORDER BY timestamp DESC
	`, startDate.Format(time.RFC3339), endDate.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("failed to query usage report: %w", err)
	}
	defer rows.Close()

	var entries []TokenUsageEntry
	for rows.Next() {
		var entry TokenUsageEntry
		var timestampStr string

		err := rows.Scan(
			&entry.ID, &entry.UserID, &entry.ConversationID, &entry.MessageID,
			&entry.TokensUsed, &entry.RequestType, &entry.Model,
			&timestampStr, &entry.ResponseTimeMs)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage entry: %w", err)
		}

		entry.Timestamp, err = time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			tm.logger.Warn("Failed to parse timestamp", "timestamp", timestampStr)
			entry.Timestamp = time.Now()
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

func (tm *TokenUsageMonitor) getUserUsage(ctx context.Context, userID int) (*UserUsage, error) {
	// Check cache first
	tm.cacheMutex.RLock()
	if cached, exists := tm.usageCache[userID]; exists {
		if time.Since(cached.LastUpdated) < tm.cacheExpiry {
			tm.cacheMutex.RUnlock()
			return cached, nil
		}
	}
	tm.cacheMutex.RUnlock()

	// Get user tier
	tier := "free" // Default tier
	err := tm.db.ReadOnly.QueryRowContext(ctx, `
		SELECT CASE WHEN is_admin = 1 THEN 'admin' ELSE 'free' END
		FROM users WHERE id = ?
	`, userID).Scan(&tier)
	if err != nil {
		tm.logger.Warn("Failed to get user tier", "user_id", userID, "error", err)
	}

	// Calculate daily usage
	today := time.Now().Format("2006-01-02")
	var dailyTokens int
	err = tm.db.ReadOnly.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(tokens_used), 0)
		FROM token_usage
		WHERE user_id = ? AND DATE(timestamp) = ?
	`, userID, today).Scan(&dailyTokens)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily usage: %w", err)
	}

	// Calculate monthly usage
	monthStart := time.Now().Format("2006-01-01")
	var monthlyTokens int
	err = tm.db.ReadOnly.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(tokens_used), 0)
		FROM token_usage
		WHERE user_id = ? AND DATE(timestamp) >= ?
	`, userID, monthStart).Scan(&monthlyTokens)
	if err != nil {
		return nil, fmt.Errorf("failed to get monthly usage: %w", err)
	}

	usage := &UserUsage{
		UserID:          userID,
		DailyTokens:     dailyTokens,
		MonthlyTokens:   monthlyTokens,
		LastUpdated:     time.Now(),
		RequestCount:    1,
		LastRequestTime: time.Now(),
		Tier:            tier,
	}

	// Update cache
	tm.cacheMutex.Lock()
	tm.usageCache[userID] = usage
	tm.cacheMutex.Unlock()

	// Clean old cache entries periodically
	if time.Since(tm.lastCacheClean) > time.Hour {
		tm.cleanCache()
	}

	return usage, nil
}

func (tm *TokenUsageMonitor) getRequestRateLimit(tier string) int {
	switch tier {
	case "admin":
		return 60 // 60 requests per minute
	case "premium":
		return 30 // 30 requests per minute
	default:
		return 10 // 10 requests per minute for free users
	}
}

func (tm *TokenUsageMonitor) getNextDayReset() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
}

func (tm *TokenUsageMonitor) getNextMonthReset() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
}

func (tm *TokenUsageMonitor) cleanCache() {
	tm.cacheMutex.Lock()
	defer tm.cacheMutex.Unlock()

	now := time.Now()
	for userID, usage := range tm.usageCache {
		if now.Sub(usage.LastUpdated) > tm.cacheExpiry {
			delete(tm.usageCache, userID)
		}
	}
	tm.lastCacheClean = now
}
