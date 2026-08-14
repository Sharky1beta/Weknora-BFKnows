package handler

import (
	"context"
	stderrors "errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/errors"
	impkg "github.com/Tencent/WeKnora/internal/im"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// MessageHandler handles HTTP requests related to messages within chat sessions
// It provides endpoints for loading and managing message history
type MessageHandler struct {
	MessageService interfaces.MessageService // Service that implements message business logic
	db             *gorm.DB
	modelService   interfaces.ModelService
	imService      *impkg.Service
}

// NewMessageHandler creates a new message handler instance with the required service
// Parameters:
//   - messageService: Service that implements message business logic
//
// Returns a pointer to a new MessageHandler
func NewMessageHandler(
	messageService interfaces.MessageService,
	db *gorm.DB,
	modelService interfaces.ModelService,
	imService *impkg.Service,
) *MessageHandler {
	return &MessageHandler{
		MessageService: messageService,
		db:             db,
		modelService:   modelService,
		imService:      imService,
	}
}

const (
	qualityDefaultWindow = 7 * 24 * time.Hour
	qualityMaxWindow     = 90 * 24 * time.Hour
)

type qualityMessageRow struct {
	ID                  string                        `gorm:"column:id"`
	SessionID           string                        `gorm:"column:session_id"`
	RequestID           string                        `gorm:"column:request_id"`
	Answer              string                        `gorm:"column:content"`
	KnowledgeReferences types.References              `gorm:"column:knowledge_references"`
	IsFallback          bool                          `gorm:"column:is_fallback"`
	AgentDurationMs     int64                         `gorm:"column:agent_duration_ms"`
	Channel             string                        `gorm:"column:channel"`
	AgentID             string                        `gorm:"column:agent_id"`
	ExecutionContext    types.MessageExecutionContext `gorm:"column:execution_context"`
	CreatedAt           time.Time                     `gorm:"column:created_at"`
	UpdatedAt           time.Time                     `gorm:"column:updated_at"`
}

type qualityQuestionRow struct {
	SessionID string    `gorm:"column:session_id"`
	RequestID string    `gorm:"column:request_id"`
	Question  string    `gorm:"column:content"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

type qualityTurn struct {
	MessageID           string    `json:"message_id"`
	SessionID           string    `json:"session_id"`
	RequestID           string    `json:"request_id"`
	Question            string    `json:"question"`
	Answer              string    `json:"answer,omitempty"`
	Channel             string    `json:"channel"`
	AgentID             string    `json:"agent_id,omitempty"`
	OccurredAt          time.Time `json:"occurred_at"`
	ResponseTimeMs      int64     `json:"response_time_ms,omitempty"`
	FirstResponseTimeMs int64     `json:"first_response_time_ms,omitempty"`
	ReferenceCount      int       `json:"reference_count"`
	Reason              string    `json:"reason,omitempty"`
	Hit                 bool      `json:"hit"`
}

type qualityChannelSummary struct {
	Total   int     `json:"total"`
	Hits    int     `json:"hits"`
	Misses  int     `json:"misses"`
	HitRate float64 `json:"hit_rate"`
}

type qualitySummary struct {
	From                       time.Time                        `json:"from"`
	To                         time.Time                        `json:"to"`
	Total                      int                              `json:"total"`
	Hits                       int                              `json:"hits"`
	Misses                     int                              `json:"misses"`
	HitRate                    float64                          `json:"hit_rate"`
	AverageResponseTimeMs      int64                            `json:"average_response_time_ms"`
	P95ResponseTimeMs          int64                            `json:"p95_response_time_ms"`
	AverageFirstResponseTimeMs int64                            `json:"average_first_response_time_ms"`
	P95FirstResponseTimeMs     int64                            `json:"p95_first_response_time_ms"`
	LatencySamples             int                              `json:"latency_samples"`
	ByChannel                  map[string]qualityChannelSummary `json:"by_channel"`
	MetricDefinition           map[string]string                `json:"metric_definition"`
}

// LoadMessages godoc
// @Summary      加载消息历史
// @Description  加载会话的消息历史，支持分页和时间筛选
// @Tags         消息
// @Accept       json
// @Produce      json
// @Param        session_id   path      string  true   "会话ID"
// @Param        limit        query     int     false  "返回数量"  default(20)
// @Param        before_time  query     string  false  "在此时间之前的消息（RFC3339Nano格式）"
// @Success      200          {object}  map[string]interface{}  "消息列表"
// @Failure      400          {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /messages/{session_id}/load [get]
func (h *MessageHandler) LoadMessages(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start loading messages")

	// Get path parameters and query parameters
	sessionID := secutils.SanitizeForLog(c.Param("session_id"))
	limit := secutils.SanitizeForLog(c.DefaultQuery("limit", "20"))
	beforeTimeStr := secutils.SanitizeForLog(c.DefaultQuery("before_time", ""))

	logger.Infof(ctx, "Loading messages params, session ID: %s, limit: %s, before time: %s",
		sessionID, limit, beforeTimeStr)

	// Parse limit parameter with fallback to default
	limitInt, err := strconv.Atoi(limit)
	if err != nil {
		logger.Warnf(ctx, "Invalid limit value, using default value 20, input: %s", limit)
		limitInt = 20
	}

	// If no beforeTime is provided, retrieve the most recent messages
	if beforeTimeStr == "" {
		logger.Infof(ctx, "Getting recent messages for session, session ID: %s, limit: %d", sessionID, limitInt)
		messages, err := h.MessageService.GetRecentMessagesBySession(ctx, sessionID, limitInt)
		if err != nil {
			if stderrors.Is(err, errors.ErrSessionNotFound) {
				// PR #1309 plumbed user-scope into the message service's
				// session existence check; non-owner / wrong-tenant lookups
				// surface as ErrSessionNotFound. Map to 404 so clients can
				// tell "wrong URL" from a real 5xx.
				logger.Warnf(ctx, "Session not found, ID: %s", sessionID)
				c.Error(errors.NewNotFoundError(err.Error()))
				return
			}
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError(err.Error()))
			return
		}

		logger.Infof(
			ctx,
			"Successfully retrieved recent messages, session ID: %s, message count: %d",
			sessionID, len(messages),
		)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    messages,
		})
		return
	}

	// If beforeTime is provided, parse the timestamp (RFC3339Nano or RFC3339).
	beforeTime, err := parseMessageBeforeTime(beforeTimeStr)
	if err != nil {
		logger.Errorf(
			ctx,
			"Invalid time format, please use RFC3339/RFC3339Nano format, err: %v, beforeTimeStr: %s",
			err, beforeTimeStr,
		)
		c.Error(errors.NewBadRequestError("Invalid time format, please use RFC3339 or RFC3339Nano format"))
		return
	}

	// Retrieve messages before the specified timestamp
	logger.Infof(ctx, "Getting messages before specific time, session ID: %s, before time: %s, limit: %d",
		sessionID, beforeTime.Format(time.RFC3339Nano), limitInt)
	messages, err := h.MessageService.GetMessagesBySessionBeforeTime(ctx, sessionID, beforeTime, limitInt)
	if err != nil {
		if stderrors.Is(err, errors.ErrSessionNotFound) {
			// See note on the GetRecentMessagesBySession path above.
			logger.Warnf(ctx, "Session not found, ID: %s", sessionID)
			c.Error(errors.NewNotFoundError(err.Error()))
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(
		ctx,
		"Successfully retrieved messages before time, session ID: %s, message count: %d",
		sessionID, len(messages),
	)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    messages,
	})
}

// DeleteMessage godoc
// @Summary      删除消息
// @Description  从会话中删除指定消息
// @Tags         消息
// @Accept       json
// @Produce      json
// @Param        session_id  path      string  true  "会话ID"
// @Param        id          path      string  true  "消息ID"
// @Success      200         {object}  map[string]interface{}  "删除成功"
// @Failure      500         {object}  errors.AppError         "服务器错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /messages/{session_id}/{id} [delete]
func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start deleting message")

	// Get path parameters for session and message identification
	sessionID := secutils.SanitizeForLog(c.Param("session_id"))
	messageID := secutils.SanitizeForLog(c.Param("id"))

	logger.Infof(ctx, "Deleting message, session ID: %s, message ID: %s", sessionID, messageID)

	// Delete the message using the message service
	if err := h.MessageService.DeleteMessage(ctx, sessionID, messageID); err != nil {
		if stderrors.Is(err, errors.ErrSessionNotFound) {
			// See note on LoadMessages above — message-service operations
			// surface ErrSessionNotFound when the caller can't see the
			// owning session (post-#1309 user scope).
			logger.Warnf(ctx, "Session not found, ID: %s", sessionID)
			c.Error(errors.NewNotFoundError(err.Error()))
			return
		}
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			// The message_id doesn't exist under this session — a client error,
			// not a server fault. 404 so callers read resource.not_found (a
			// permanent condition, not retryable) instead of a 5xx. Mirrors the
			// ContinueStream / kb / doc / chunk not-found handling.
			logger.Warnf(ctx, "Message not found, session ID: %s, message ID: %s", sessionID, messageID)
			c.Error(errors.NewNotFoundError(err.Error()))
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Message deleted successfully, session ID: %s, message ID: %s", sessionID, messageID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Message deleted successfully",
	})
}

// SearchMessages godoc
// @Summary      搜索历史对话
// @Description  通过关键词和/或向量相似度搜索历史对话记录，支持关键词、向量、混合三种模式
// @Tags         消息
// @Accept       json
// @Produce      json
// @Param        request  body      SearchMessagesRequest  true  "搜索请求"
// @Success      200      {object}  map[string]interface{}  "搜索结果"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /messages/search [post]
func (h *MessageHandler) SearchMessages(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start searching messages")

	var request SearchMessagesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		logger.Error(ctx, "Failed to parse search request", err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	if request.Query == "" {
		logger.Error(ctx, "Query content is empty")
		c.Error(errors.NewBadRequestError("Query content cannot be empty"))
		return
	}

	params := &types.MessageSearchParams{
		Query:      secutils.SanitizeForLog(request.Query),
		Mode:       types.MessageSearchMode(request.Mode),
		Limit:      request.Limit,
		SessionIDs: request.SessionIDs,
	}

	logger.Infof(ctx, "Searching messages with params: query=%s, mode=%s, limit=%d, session_ids=%v",
		params.Query, params.Mode, params.Limit, params.SessionIDs)

	result, err := h.MessageService.SearchMessages(ctx, params)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Message search completed, found %d results", result.Total)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// SearchMessagesRequest defines the request structure for searching messages
type SearchMessagesRequest struct {
	// Query text for search
	Query string `json:"query" binding:"required"`
	// Search mode: "keyword", "vector", "hybrid" (default: "hybrid")
	Mode string `json:"mode"`
	// Maximum number of results to return (default: 20)
	Limit int `json:"limit"`
	// Filter by specific session IDs (optional)
	SessionIDs []string `json:"session_ids"`
}

// GetChatHistoryKBStats godoc
// @Summary      获取聊天历史知识库统计
// @Description  获取聊天历史知识库的统计信息（已索引消息数、知识库大小等）
// @Tags         消息
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "统计信息"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /messages/chat-history-stats [get]
func (h *MessageHandler) GetChatHistoryKBStats(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Getting chat history KB stats")

	stats, err := h.MessageService.GetChatHistoryKBStats(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// parseMessageBeforeTime parses the `before_time` query used by LoadMessages.
// Frontend cursors may be RFC3339 (no fractional seconds) or RFC3339Nano.
func parseMessageBeforeTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, stderrors.New("empty before_time")
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339}
	var lastErr error
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}

// GetQualitySummary returns tenant-level knowledge Q&A quality metrics.
// A turn is monitored when it explicitly used a knowledge scope, returned a
// knowledge reference, or was marked as a knowledge fallback. This keeps
// ordinary small talk out of the hit-rate denominator.
func (h *MessageHandler) GetQualitySummary(c *gin.Context) {
	ctx := c.Request.Context()
	from, to, err := parseQualityWindow(c.Query("from"), c.Query("to"), time.Now())
	if err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	turns, err := h.loadQualityTurns(ctx, from, to, c.Query("agent_id"), c.Query("channel"))
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	summary := summarizeQualityTurns(turns, from, to)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

// ListQualityMisses returns the original questions for knowledge turns that
// fell back, completed without a knowledge reference, or explicitly stated
// that the retrieved knowledge could not answer the question.
func (h *MessageHandler) ListQualityMisses(c *gin.Context) {
	ctx := c.Request.Context()
	from, to, err := parseQualityWindow(c.Query("from"), c.Query("to"), time.Now())
	if err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	page, pageSize, err := parseQualityPagination(c.Query("page"), c.Query("page_size"))
	if err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	turns, err := h.loadQualityTurns(ctx, from, to, c.Query("agent_id"), c.Query("channel"))
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	misses := make([]qualityTurn, 0)
	for _, turn := range turns {
		if !turn.Hit {
			misses = append(misses, turn)
		}
	}
	total := len(misses)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":     misses[start:end],
			"total":     total,
			"page":      page,
			"page_size": pageSize,
			"from":      from,
			"to":        to,
		},
	})
}

func (h *MessageHandler) loadQualityTurns(
	ctx context.Context, from, to time.Time, agentID, channel string,
) ([]qualityTurn, error) {
	tenantID := types.MustTenantIDFromContext(ctx)

	var answers []qualityMessageRow
	query := h.db.WithContext(ctx).
		Table("messages AS a").
		Select("a.id, a.session_id, a.request_id, a.content, a.knowledge_references, a.is_fallback, "+
			"a.agent_duration_ms, a.channel, a.agent_id, a.execution_context, a.created_at, a.updated_at").
		Joins("INNER JOIN sessions AS s ON s.id = a.session_id AND s.deleted_at IS NULL").
		Where("s.tenant_id = ?", tenantID).
		Where("a.deleted_at IS NULL AND a.role = ? AND a.is_completed = ?", "assistant", true).
		Where("a.created_at >= ? AND a.created_at < ?", from, to)
	if agentID = strings.TrimSpace(agentID); agentID != "" {
		query = query.Where("a.agent_id = ?", agentID)
	}
	if channel = strings.TrimSpace(channel); channel != "" {
		query = query.Where("a.channel = ?", channel)
	}
	if err := query.Order("a.created_at DESC").Find(&answers).Error; err != nil {
		return nil, err
	}

	requestIDs := make([]string, 0, len(answers))
	for _, answer := range answers {
		if isMonitoredQualityAnswer(answer) && answer.RequestID != "" {
			requestIDs = append(requestIDs, answer.RequestID)
		}
	}

	questions := make(map[string]qualityQuestionRow, len(requestIDs))
	if len(requestIDs) > 0 {
		var rows []qualityQuestionRow
		if err := h.db.WithContext(ctx).
			Table("messages AS q").
			Select("q.session_id, q.request_id, q.content, q.created_at").
			Joins("INNER JOIN sessions AS s ON s.id = q.session_id AND s.deleted_at IS NULL").
			Where("s.tenant_id = ?", tenantID).
			Where("q.deleted_at IS NULL AND q.role = ? AND q.request_id IN ?", "user", requestIDs).
			Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			questions[qualityPairKey(row.SessionID, row.RequestID)] = row
		}
	}

	turns := make([]qualityTurn, 0, len(answers))
	for _, answer := range answers {
		if !isMonitoredQualityAnswer(answer) {
			continue
		}
		question := questions[qualityPairKey(answer.SessionID, answer.RequestID)]
		hit, reason := classifyQualityAnswer(answer)
		responseTime := int64(0)
		if !question.CreatedAt.IsZero() && !answer.UpdatedAt.Before(question.CreatedAt) {
			responseTime = answer.UpdatedAt.Sub(question.CreatedAt).Milliseconds()
		}
		turns = append(turns, qualityTurn{
			MessageID:           answer.ID,
			SessionID:           answer.SessionID,
			RequestID:           answer.RequestID,
			Question:            question.Question,
			Answer:              answer.Answer,
			Channel:             normalizedQualityChannel(answer.Channel),
			AgentID:             answer.AgentID,
			OccurredAt:          answer.CreatedAt,
			ResponseTimeMs:      responseTime,
			FirstResponseTimeMs: answer.AgentDurationMs,
			ReferenceCount:      len(answer.KnowledgeReferences),
			Reason:              reason,
			Hit:                 hit,
		})
	}
	return turns, nil
}

func isMonitoredQualityAnswer(answer qualityMessageRow) bool {
	return answer.IsFallback || len(answer.KnowledgeReferences) > 0 ||
		len(answer.ExecutionContext.KnowledgeBaseIDs) > 0 ||
		len(answer.ExecutionContext.KnowledgeIDs) > 0 ||
		len(answer.ExecutionContext.TagIDs) > 0
}

func classifyQualityAnswer(answer qualityMessageRow) (bool, string) {
	if answer.IsFallback {
		return false, "fallback"
	}
	if len(answer.KnowledgeReferences) == 0 {
		return false, "no_knowledge_reference"
	}
	if isExplicitKnowledgeMiss(answer.Answer) {
		return false, "explicit_no_answer"
	}
	return true, ""
}

// isExplicitKnowledgeMiss recognizes deterministic abstention language in the
// persisted answer. It deliberately requires either a strong inability phrase
// or both a knowledge-source marker and an absence phrase, reducing false
// positives from ordinary sentences that happen to contain words such as
// "没有". No model call is made for this classification.
func isExplicitKnowledgeMiss(answer string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(answer), " "))
	if normalized == "" {
		return false
	}
	strongMissPhrases := []string{
		"无法回答", "不能回答", "无法确定", "无法得知", "无法告知", "无法提供答案",
		"cannot answer", "can't answer", "unable to answer", "not enough information",
		"insufficient information",
	}
	if containsQualityPhrase(normalized, strongMissPhrases) {
		return true
	}
	knowledgeSourcePhrases := []string{
		"知识库", "检索", "搜索结果", "提供的信息", "提供的内容", "引用内容",
		"knowledge base", "retrieved", "search result", "provided context", "provided information",
	}
	absencePhrases := []string{
		"未找到", "没有找到", "未检索到", "没有检索到", "不包含", "未包含", "并未包含",
		"没有相关", "无相关", "does not contain", "did not contain", "no relevant",
		"could not find", "couldn't find", "not found",
	}
	return containsQualityPhrase(normalized, knowledgeSourcePhrases) &&
		containsQualityPhrase(normalized, absencePhrases)
}

func containsQualityPhrase(text string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func summarizeQualityTurns(turns []qualityTurn, from, to time.Time) qualitySummary {
	summary := qualitySummary{
		From:      from,
		To:        to,
		Total:     len(turns),
		ByChannel: make(map[string]qualityChannelSummary),
		MetricDefinition: map[string]string{
			"monitored_turn": "knowledge scope selected, knowledge reference returned, or fallback triggered",
			"hit":            "at least one knowledge reference, no fallback, and no explicit no-answer statement",
			"miss":           "fallback, no knowledge reference, or an explicit statement that retrieved knowledge cannot answer",
			"response_time":  "milliseconds from the user message creation to completed assistant-message update",
			"first_response": "stored agent_duration_ms; omitted from latency aggregates when unavailable",
		},
	}
	responseTimes := make([]int64, 0, len(turns))
	firstResponseTimes := make([]int64, 0, len(turns))
	for _, turn := range turns {
		channel := normalizedQualityChannel(turn.Channel)
		byChannel := summary.ByChannel[channel]
		byChannel.Total++
		if turn.Hit {
			summary.Hits++
			byChannel.Hits++
		} else {
			summary.Misses++
			byChannel.Misses++
		}
		if turn.ResponseTimeMs > 0 {
			responseTimes = append(responseTimes, turn.ResponseTimeMs)
		}
		if turn.FirstResponseTimeMs > 0 {
			firstResponseTimes = append(firstResponseTimes, turn.FirstResponseTimeMs)
		}
		summary.ByChannel[channel] = byChannel
	}
	if summary.Total > 0 {
		summary.HitRate = roundQualityRate(float64(summary.Hits) * 100 / float64(summary.Total))
	}
	for channel, item := range summary.ByChannel {
		if item.Total > 0 {
			item.HitRate = roundQualityRate(float64(item.Hits) * 100 / float64(item.Total))
		}
		summary.ByChannel[channel] = item
	}
	summary.LatencySamples = len(responseTimes)
	summary.AverageResponseTimeMs = averageQualityDuration(responseTimes)
	summary.P95ResponseTimeMs = percentileQualityDuration(responseTimes, 0.95)
	summary.AverageFirstResponseTimeMs = averageQualityDuration(firstResponseTimes)
	summary.P95FirstResponseTimeMs = percentileQualityDuration(firstResponseTimes, 0.95)
	return summary
}

func parseQualityWindow(rawFrom, rawTo string, now time.Time) (time.Time, time.Time, error) {
	to := now.UTC()
	from := to.Add(-qualityDefaultWindow)
	var err error
	if strings.TrimSpace(rawTo) != "" {
		to, err = time.Parse(time.RFC3339, strings.TrimSpace(rawTo))
		if err != nil {
			return time.Time{}, time.Time{}, stderrors.New("to must be RFC3339")
		}
	}
	if strings.TrimSpace(rawFrom) != "" {
		from, err = time.Parse(time.RFC3339, strings.TrimSpace(rawFrom))
		if err != nil {
			return time.Time{}, time.Time{}, stderrors.New("from must be RFC3339")
		}
	} else {
		from = to.Add(-qualityDefaultWindow)
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, stderrors.New("from must be before to")
	}
	if to.Sub(from) > qualityMaxWindow {
		return time.Time{}, time.Time{}, stderrors.New("time range cannot exceed 90 days")
	}
	return from.UTC(), to.UTC(), nil
}

func parseQualityPagination(rawPage, rawPageSize string) (int, int, error) {
	page, pageSize := 1, 50
	var err error
	if strings.TrimSpace(rawPage) != "" {
		page, err = strconv.Atoi(rawPage)
		if err != nil || page < 1 {
			return 0, 0, stderrors.New("page must be a positive integer")
		}
	}
	if strings.TrimSpace(rawPageSize) != "" {
		pageSize, err = strconv.Atoi(rawPageSize)
		if err != nil || pageSize < 1 || pageSize > 200 {
			return 0, 0, stderrors.New("page_size must be between 1 and 200")
		}
	}
	return page, pageSize, nil
}

func qualityPairKey(sessionID, requestID string) string { return sessionID + "\x00" + requestID }

func normalizedQualityChannel(channel string) string {
	channel = strings.TrimSpace(strings.ToLower(channel))
	if channel == "" {
		return "unknown"
	}
	return channel
}

func averageQualityDuration(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	var total int64
	for _, value := range values {
		total += value
	}
	return total / int64(len(values))
}

func percentileQualityDuration(values []int64, percentile float64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	return sorted[index]
}

func roundQualityRate(value float64) float64 { return math.Round(value*100) / 100 }
