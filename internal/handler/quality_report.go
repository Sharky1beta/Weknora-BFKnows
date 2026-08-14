package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

const qualityReportSchedule = "weekly_monday_0900"

var qualityReportWeekdays = map[string]time.Weekday{
	"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday,
	"wednesday": time.Wednesday, "thursday": time.Thursday,
	"friday": time.Friday, "saturday": time.Saturday,
}

type qualityReportConfig struct {
	TenantID       uint64     `json:"tenant_id" gorm:"primaryKey"`
	Enabled        bool       `json:"enabled"`
	WindowDays     int        `json:"window_days"`
	ModelID        string     `json:"model_id"`
	Schedule       string     `json:"schedule"`
	Timezone       string     `json:"timezone"`
	IMChannelID    string     `json:"im_channel_id"`
	TargetChatID   string     `json:"target_chat_id"`
	TargetChatName string     `json:"target_chat_name"`
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (qualityReportConfig) TableName() string { return "quality_report_configs" }

type qualityReportRecord struct {
	ID         string     `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID   uint64     `json:"tenant_id"`
	WindowFrom time.Time  `json:"window_from"`
	WindowTo   time.Time  `json:"window_to"`
	Status     string     `json:"status"`
	ModelID    string     `json:"model_id"`
	Report     types.JSON `json:"report" gorm:"type:jsonb"`
	SentAt     *time.Time `json:"sent_at,omitempty"`
	SendError  string     `json:"send_error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (qualityReportRecord) TableName() string { return "quality_reports" }

type qualityQuestionFrequency struct {
	Question string `json:"question"`
	Count    int    `json:"count"`
	Hits     int    `json:"hits"`
	Misses   int    `json:"misses"`
}

type qualityGapCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type qualityAIAnalysis struct {
	ExecutiveSummary string   `json:"executive_summary"`
	KeyFindings      []string `json:"key_findings"`
	Recommendations  []string `json:"recommendations"`
	FurtherQuestions []string `json:"further_questions"`
	Caveats          []string `json:"caveats"`
}

type qualityReportPayload struct {
	Title             string                     `json:"title"`
	GeneratedAt       time.Time                  `json:"generated_at"`
	Current           qualitySummary             `json:"current"`
	Previous          qualitySummary             `json:"previous"`
	HitRateChange     float64                    `json:"hit_rate_change"`
	FrequentQuestions []qualityQuestionFrequency `json:"frequent_questions"`
	GapByReason       []qualityGapCount          `json:"gap_by_reason"`
	GapByChannel      []qualityGapCount          `json:"gap_by_channel"`
	Analysis          qualityAIAnalysis          `json:"analysis"`
	ModelID           string                     `json:"model_id"`
}

type generateQualityReportRequest struct {
	WindowDays int    `json:"window_days"`
	ModelID    string `json:"model_id"`
}

type updateQualityReportConfigRequest struct {
	Enabled        bool   `json:"enabled"`
	WindowDays     int    `json:"window_days"`
	ModelID        string `json:"model_id"`
	Schedule       string `json:"schedule"`
	IMChannelID    string `json:"im_channel_id"`
	TargetChatID   string `json:"target_chat_id"`
	TargetChatName string `json:"target_chat_name"`
}

func defaultQualityReportConfig(tenantID uint64) qualityReportConfig {
	return qualityReportConfig{
		TenantID: tenantID, Enabled: false, WindowDays: 7,
		Schedule: qualityReportSchedule, Timezone: "Asia/Shanghai",
	}
}

func (h *MessageHandler) GetQualityReportConfig(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := types.MustTenantIDFromContext(ctx)
	config, err := h.loadQualityReportConfig(ctx, tenantID)
	if err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": config})
}

func (h *MessageHandler) UpdateQualityReportConfig(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := types.MustTenantIDFromContext(ctx)
	var req updateQualityReportConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	if req.WindowDays == 0 {
		req.WindowDays = 7
	}
	if req.WindowDays < 1 || req.WindowDays > 90 {
		c.Error(errors.NewBadRequestError("window_days must be between 1 and 90"))
		return
	}
	if req.Enabled && (strings.TrimSpace(req.IMChannelID) == "" || strings.TrimSpace(req.TargetChatID) == "") {
		c.Error(errors.NewBadRequestError("automatic sending requires an IM channel and target group"))
		return
	}
	config, err := h.loadQualityReportConfig(ctx, tenantID)
	if err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	config.Enabled = req.Enabled
	config.WindowDays = req.WindowDays
	config.ModelID = strings.TrimSpace(req.ModelID)
	schedule, err := normalizeQualityReportSchedule(req.Schedule)
	if err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	config.IMChannelID = strings.TrimSpace(req.IMChannelID)
	config.TargetChatID = strings.TrimSpace(req.TargetChatID)
	config.TargetChatName = strings.TrimSpace(req.TargetChatName)
	config.Schedule = schedule
	config.Timezone = "Asia/Shanghai"
	if err := h.db.WithContext(ctx).Save(&config).Error; err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": config})
}

func (h *MessageHandler) GenerateQualityReport(c *gin.Context) {
	ctx := c.Request.Context()
	var req generateQualityReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	if req.WindowDays == 0 {
		req.WindowDays = 7
	}
	record, payload, err := h.generateQualityReport(ctx, req.WindowDays, strings.TrimSpace(req.ModelID), time.Now())
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		if record != nil && payload != nil {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"record": record, "report": payload}})
			return
		}
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"record": record, "report": payload}})
}

func (h *MessageHandler) ListQualityReports(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := types.MustTenantIDFromContext(ctx)
	var rows []qualityReportRecord
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tenantID).
		Order("created_at DESC").Limit(20).Find(&rows).Error; err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

func (h *MessageHandler) SendQualityReport(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := types.MustTenantIDFromContext(ctx)
	var record qualityReportRecord
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", c.Param("id"), tenantID).First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("quality report not found"))
			return
		}
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	config, err := h.loadQualityReportConfig(ctx, tenantID)
	if err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	if err := h.sendQualityReport(ctx, &record, config); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": record})
}

func (h *MessageHandler) loadQualityReportConfig(ctx context.Context, tenantID uint64) (qualityReportConfig, error) {
	config := defaultQualityReportConfig(tenantID)
	err := h.db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&config).Error
	if err == gorm.ErrRecordNotFound {
		return config, nil
	}
	return config, err
}

func (h *MessageHandler) generateQualityReport(
	ctx context.Context, windowDays int, modelID string, now time.Time,
) (*qualityReportRecord, *qualityReportPayload, error) {
	if windowDays < 1 || windowDays > 90 {
		return nil, nil, fmt.Errorf("window_days must be between 1 and 90")
	}
	to := now.UTC()
	from := to.Add(-time.Duration(windowDays) * 24 * time.Hour)
	previousFrom := from.Add(-time.Duration(windowDays) * 24 * time.Hour)
	currentTurns, err := h.loadQualityTurns(ctx, from, to, "", "")
	if err != nil {
		return nil, nil, err
	}
	previousTurns, err := h.loadQualityTurns(ctx, previousFrom, from, "", "")
	if err != nil {
		return nil, nil, err
	}
	current := summarizeQualityTurns(currentTurns, from, to)
	previous := summarizeQualityTurns(previousTurns, previousFrom, from)
	frequent, reasonGaps, channelGaps := aggregateQualityReport(currentTurns)

	model, resolvedModelID, err := h.resolveQualityReportModel(ctx, modelID)
	if err != nil {
		return nil, nil, err
	}
	analysis, err := generateQualityAIAnalysis(ctx, model, current, previous, frequent, reasonGaps, channelGaps)
	if err != nil {
		return h.createFailedQualityReport(ctx, from, to, current, previous, frequent, reasonGaps, channelGaps, resolvedModelID, fmt.Errorf("generate AI recommendations: %w", err))
	}
	payload := &qualityReportPayload{
		Title: "知识问答质量周报", GeneratedAt: to, Current: current, Previous: previous,
		HitRateChange:     current.HitRate - previous.HitRate,
		FrequentQuestions: frequent, GapByReason: reasonGaps, GapByChannel: channelGaps,
		Analysis: analysis, ModelID: resolvedModelID,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	record := &qualityReportRecord{
		ID: uuid.NewString(), TenantID: types.MustTenantIDFromContext(ctx),
		WindowFrom: from, WindowTo: to, Status: "generated", ModelID: resolvedModelID,
		Report: types.JSON(raw),
	}
	if err := h.db.WithContext(ctx).Create(record).Error; err != nil {
		return nil, nil, err
	}
	return record, payload, nil
}

func (h *MessageHandler) createFailedQualityReport(
	ctx context.Context,
	from time.Time,
	to time.Time,
	current qualitySummary,
	previous qualitySummary,
	frequent []qualityQuestionFrequency,
	reasonGaps []qualityGapCount,
	channelGaps []qualityGapCount,
	modelID string,
	cause error,
) (*qualityReportRecord, *qualityReportPayload, error) {
	message := strings.TrimSpace(cause.Error())
	payload := &qualityReportPayload{
		Title:             "知识问答质量周报",
		GeneratedAt:       to,
		Current:           current,
		Previous:          previous,
		HitRateChange:     current.HitRate - previous.HitRate,
		FrequentQuestions: frequent,
		GapByReason:       reasonGaps,
		GapByChannel:      channelGaps,
		Analysis: qualityAIAnalysis{
			ExecutiveSummary: "AI 分析生成失败，报告未完成；已保留本次统计数据与失败原因。",
			KeyFindings:      []string{"报告基础统计已生成，但 AI 分析未完成。"},
			Recommendations:  []string{},
			FurtherQuestions: []string{},
			Caveats:          []string{message},
		},
		ModelID: modelID,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	record := &qualityReportRecord{
		ID: uuid.NewString(), TenantID: types.MustTenantIDFromContext(ctx),
		WindowFrom: from, WindowTo: to, Status: "failed", ModelID: modelID,
		Report: types.JSON(raw), SendError: message,
	}
	if err := h.db.WithContext(ctx).Create(record).Error; err != nil {
		return nil, nil, err
	}
	logger.Warnf(ctx, "[quality-report] generated failed report id=%s model=%s error=%s", record.ID, modelID, message)
	return record, payload, cause
}

func (h *MessageHandler) resolveQualityReportModel(ctx context.Context, requested string) (chat.Chat, string, error) {
	models, err := h.modelService.ListModels(ctx)
	if err != nil {
		return nil, "", err
	}
	requested = strings.TrimSpace(requested)
	var selected string
	for _, model := range models {
		if requested != "" && model.ID == requested {
			selected = model.ID
			break
		}
	}
	if requested != "" && selected == "" {
		return nil, "", fmt.Errorf("selected report model is unavailable")
	}
	if selected == "" {
		for _, model := range models {
			if model.Type == types.ModelTypeKnowledgeQA && model.IsDefault && model.Status == types.ModelStatusActive {
				selected = model.ID
				break
			}
		}
	}
	if selected == "" {
		for _, model := range models {
			if model.Type == types.ModelTypeKnowledgeQA && model.Status == types.ModelStatusActive {
				selected = model.ID
				break
			}
		}
	}
	if selected == "" {
		return nil, "", fmt.Errorf("no active knowledge QA model is available for report generation")
	}
	instance, err := h.modelService.GetChatModel(ctx, selected)
	return instance, selected, err
}

func aggregateQualityReport(turns []qualityTurn) ([]qualityQuestionFrequency, []qualityGapCount, []qualityGapCount) {
	type counter struct {
		display             string
		total, hits, misses int
	}
	questions := map[string]*counter{}
	reasons := map[string]int{}
	channels := map[string]int{}
	for _, turn := range turns {
		key := strings.ToLower(strings.Join(strings.Fields(turn.Question), " "))
		if key != "" {
			item := questions[key]
			if item == nil {
				item = &counter{display: strings.TrimSpace(turn.Question)}
				questions[key] = item
			}
			item.total++
			if turn.Hit {
				item.hits++
			} else {
				item.misses++
			}
		}
		if !turn.Hit {
			reasons[turn.Reason]++
			channels[turn.Channel]++
		}
	}
	frequent := make([]qualityQuestionFrequency, 0, len(questions))
	for _, item := range questions {
		frequent = append(frequent, qualityQuestionFrequency{Question: item.display, Count: item.total, Hits: item.hits, Misses: item.misses})
	}
	sort.Slice(frequent, func(i, j int) bool {
		if frequent[i].Count == frequent[j].Count {
			return frequent[i].Misses > frequent[j].Misses
		}
		return frequent[i].Count > frequent[j].Count
	})
	if len(frequent) > 10 {
		frequent = frequent[:10]
	}
	return frequent, sortedGapCounts(reasons), sortedGapCounts(channels)
}

func sortedGapCounts(values map[string]int) []qualityGapCount {
	items := make([]qualityGapCount, 0, len(values))
	for name, count := range values {
		items = append(items, qualityGapCount{Name: name, Count: count})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Count > items[j].Count })
	return items
}

func generateQualityAIAnalysis(
	ctx context.Context, model chat.Chat, current, previous qualitySummary,
	frequent []qualityQuestionFrequency, reasons, channels []qualityGapCount,
) (qualityAIAnalysis, error) {
	evidence, _ := json.Marshal(map[string]any{
		"current": current, "previous": previous, "frequent_questions": frequent,
		"gap_by_reason": reasons, "gap_by_channel": channels,
	})
	messages := []chat.Message{
		{Role: "system", Content: `你是企业知识库质量分析师。只依据给定数据生成简洁、可执行的中文周报分析。不要编造原因、负责人或目标值。输出严格 JSON，不要 Markdown：{"executive_summary":"一句话结论","key_findings":["发现"],"recommendations":["具体行动"],"further_questions":["需要进一步确认的问题"],"caveats":["数据局限"]}`},
		{Role: "user", Content: "请分析以下知识问答质量数据：\n" + string(evidence)},
	}
	var response *types.ChatResponse
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		response, err = model.Chat(ctx, messages, &chat.ChatOptions{Temperature: 0.2, MaxTokens: 1200})
		if err == nil {
			break
		}
		if attempt == 3 || ctx.Err() != nil || !isTransientQualityAIError(err) {
			logger.Warnf(ctx, "[quality-report] AI recommendations unavailable, using fallback analysis: %v", err)
			return buildQualityFallbackAnalysis(current, previous, frequent, reasons, channels, err), nil
		}
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
	raw := strings.TrimSpace(response.Content)
	preview := qualityReportPreview(raw, 500)
	suffix := qualityReportSuffix(raw, 500)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	if start := strings.Index(raw, "{"); start > 0 {
		raw = raw[start:]
	}
	if end := strings.LastIndex(raw, "}"); end >= 0 && end < len(raw)-1 {
		raw = raw[:end+1]
	}
	var result qualityAIAnalysis
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		logger.Warnf(ctx, "[quality-report] AI JSON parse failed response_length=%d preview=%q suffix=%q error=%v", len(response.Content), preview, suffix, err)
		return buildQualityFallbackAnalysis(current, previous, frequent, reasons, channels, fmt.Errorf("parse AI recommendations JSON: %w; response_length=%d", err, len(response.Content))), nil
	}
	if strings.TrimSpace(result.ExecutiveSummary) == "" || len(result.Recommendations) == 0 {
		logger.Warnf(ctx, "[quality-report] AI recommendations incomplete response_length=%d summary_empty=%t recommendations=%d preview=%q suffix=%q", len(response.Content), strings.TrimSpace(result.ExecutiveSummary) == "", len(result.Recommendations), preview, suffix)
		return buildQualityFallbackAnalysis(current, previous, frequent, reasons, channels, fmt.Errorf("AI recommendations are incomplete: summary_empty=%t recommendations=%d response_length=%d", strings.TrimSpace(result.ExecutiveSummary) == "", len(result.Recommendations), len(response.Content))), nil
	}
	return result, nil
}

func buildQualityFallbackAnalysis(
	current qualitySummary,
	previous qualitySummary,
	frequent []qualityQuestionFrequency,
	reasons []qualityGapCount,
	channels []qualityGapCount,
	cause error,
) qualityAIAnalysis {
	change := current.HitRate - previous.HitRate
	summary := fmt.Sprintf("本期共 %d 次知识问答，命中率 %.1f%%，未命中 %d 次，较上期 %+.1f 个百分点。", current.Total, current.HitRate, current.Misses, change)
	findings := []string{
		fmt.Sprintf("本期知识问答命中 %d 次、未命中 %d 次。", current.Hits, current.Misses),
	}
	if len(frequent) > 0 {
		findings = append(findings, fmt.Sprintf("高频问题 Top1：%s（出现 %d 次，未命中 %d 次）。", frequent[0].Question, frequent[0].Count, frequent[0].Misses))
	}
	if len(reasons) > 0 {
		findings = append(findings, fmt.Sprintf("主要缺口类型：%s（%d 次）。", reasons[0].Name, reasons[0].Count))
	}
	if len(channels) > 0 {
		findings = append(findings, fmt.Sprintf("主要问题渠道：%s（%d 次）。", channels[0].Name, channels[0].Count))
	}
	recommendations := []string{
		"优先补充高频未命中问题对应的知识条目，并在补充后复测命中情况。",
		"对未命中问题按原因分组，分别处理兜底回答、无知识引用和引用无效等问题。",
	}
	if len(frequent) > 0 {
		recommendations = append(recommendations, fmt.Sprintf("围绕“%s”补充标准问答、业务说明或操作指引。", frequent[0].Question))
	}
	if len(reasons) > 0 {
		recommendations = append(recommendations, fmt.Sprintf("针对“%s”类缺口建立专项补充清单，完成后观察下期命中率变化。", reasons[0].Name))
	}
	caveats := []string{"AI 模型未能返回可用结构化分析，本报告使用规则兜底生成建议。"}
	if cause != nil {
		caveats = append(caveats, cause.Error())
	}
	return qualityAIAnalysis{
		ExecutiveSummary: summary,
		KeyFindings:      findings,
		Recommendations:  recommendations,
		FurtherQuestions: []string{"哪些高频未命中问题需要指定负责人补充？", "补充后的知识是否已被对应智能体索引并命中？"},
		Caveats:          caveats,
	}
}

func qualityReportPreview(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func qualityReportSuffix(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	return string(runes[len(runes)-limit:])
}

func isTransientQualityAIError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	transientPhrases := []string{
		"timeout",
		"tls handshake timeout",
		"connection reset",
		"connection refused",
		"temporary",
		"too many requests",
		"status code: 429",
		"status code: 500",
		"status code: 502",
		"status code: 503",
		"status code: 504",
	}
	for _, phrase := range transientPhrases {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}

func (h *MessageHandler) sendQualityReport(ctx context.Context, record *qualityReportRecord, config qualityReportConfig) error {
	if record.Status != "generated" && record.Status != "send_failed" {
		return fmt.Errorf("quality report is not ready to send: %s", record.Status)
	}
	if strings.TrimSpace(config.IMChannelID) == "" || strings.TrimSpace(config.TargetChatID) == "" {
		return fmt.Errorf("project management group is not configured")
	}
	var payload qualityReportPayload
	if err := json.Unmarshal(record.Report, &payload); err != nil {
		return err
	}
	content := formatQualityReportMessage(payload)
	err := h.imService.SendProactiveGroupMessage(ctx, config.IMChannelID, config.TargetChatID, content)
	now := time.Now().UTC()
	updates := map[string]any{"updated_at": now}
	if err != nil {
		updates["status"] = "send_failed"
		updates["send_error"] = err.Error()
	} else {
		updates["status"] = "sent"
		updates["sent_at"] = now
		updates["send_error"] = ""
		record.Status = "sent"
		record.SentAt = &now
		record.SendError = ""
	}
	_ = h.db.WithContext(ctx).Model(record).Updates(updates).Error
	return err
}

func formatQualityReportMessage(report qualityReportPayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", report.Title)
	fmt.Fprintf(&b, "统计周期：%s 至 %s\n\n", report.Current.From.Format("2006-01-02"), report.Current.To.Format("2006-01-02"))
	fmt.Fprintf(&b, "**摘要**：%s\n\n", report.Analysis.ExecutiveSummary)
	fmt.Fprintf(&b, "- 问答总数：%d\n- 命中率：%.1f%%（较上期 %+.1f 个百分点）\n- 未命中：%d\n- 平均响应：%.2f 秒\n",
		report.Current.Total, report.Current.HitRate, report.HitRateChange, report.Current.Misses, float64(report.Current.AverageResponseTimeMs)/1000)
	if len(report.Analysis.KeyFindings) > 0 {
		b.WriteString("\n**关键发现**\n")
		for _, item := range report.Analysis.KeyFindings {
			fmt.Fprintf(&b, "- %s\n", item)
		}
	}
	if len(report.Analysis.Recommendations) > 0 {
		b.WriteString("\n**改进建议**\n")
		for _, item := range report.Analysis.Recommendations {
			fmt.Fprintf(&b, "- %s\n", item)
		}
	}
	return b.String()
}

// StartQualityReportScheduler starts the lightweight weekly report runner.
// Disabled or incomplete configurations are ignored, so a blank target group
// never produces an outbound message.
func (h *MessageHandler) StartQualityReportScheduler(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				h.runDueQualityReports(ctx, now)
			}
		}
	}()
}

func (h *MessageHandler) runDueQualityReports(ctx context.Context, now time.Time) {
	var configs []qualityReportConfig
	if err := h.db.WithContext(ctx).Where("enabled = ? AND im_channel_id <> '' AND target_chat_id <> ''", true).Find(&configs).Error; err != nil {
		logger.Warnf(ctx, "[quality-report] load configs: %v", err)
		return
	}
	for _, config := range configs {
		loc, err := time.LoadLocation(config.Timezone)
		if err != nil {
			loc = time.FixedZone("CST", 8*3600)
		}
		localNow := now.In(loc)
		startOfSlot, due := qualityReportScheduleSlot(config.Schedule, localNow)
		if !due {
			continue
		}
		if config.LastRunAt != nil && !config.LastRunAt.Before(startOfSlot) {
			continue
		}
		tenantCtx := context.WithValue(ctx, types.TenantIDContextKey, config.TenantID)
		record, _, genErr := h.generateQualityReport(tenantCtx, config.WindowDays, config.ModelID, now)
		if genErr == nil {
			genErr = h.sendQualityReport(tenantCtx, record, config)
		}
		if genErr != nil {
			logger.Warnf(ctx, "[quality-report] tenant=%d: %v", config.TenantID, genErr)
			continue
		}
		attempted := time.Now().UTC()
		_ = h.db.WithContext(ctx).Model(&qualityReportConfig{}).Where("tenant_id = ?", config.TenantID).Update("last_run_at", attempted).Error
	}
}

func normalizeQualityReportSchedule(schedule string) (string, error) {
	schedule = strings.ToLower(strings.TrimSpace(schedule))
	if schedule == "" {
		return qualityReportSchedule, nil
	}
	parts := strings.Split(schedule, "_")
	if len(parts) != 3 || parts[0] != "weekly" {
		return "", fmt.Errorf("unsupported quality report schedule: %s", schedule)
	}
	if _, ok := qualityReportWeekdays[parts[1]]; !ok {
		return "", fmt.Errorf("unsupported quality report weekday: %s", parts[1])
	}
	if len(parts[2]) != 4 {
		return "", fmt.Errorf("invalid quality report time: %s", parts[2])
	}
	hour, hourErr := strconv.Atoi(parts[2][:2])
	minute, minuteErr := strconv.Atoi(parts[2][2:])
	if hourErr != nil || minuteErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return "", fmt.Errorf("invalid quality report time: %s", parts[2])
	}
	return fmt.Sprintf("weekly_%s_%02d%02d", parts[1], hour, minute), nil
}

// qualityReportScheduleSlot returns the UTC start of the current due slot.
// The scheduler checks once per minute, while LastRunAt prevents duplicate
// sends during the selected hour.
func qualityReportScheduleSlot(schedule string, localNow time.Time) (time.Time, bool) {
	normalized, err := normalizeQualityReportSchedule(schedule)
	if err != nil {
		return time.Time{}, false
	}
	parts := strings.Split(normalized, "_")
	weekday := qualityReportWeekdays[parts[1]]
	hour, _ := strconv.Atoi(parts[2][:2])
	minute, _ := strconv.Atoi(parts[2][2:])
	if localNow.Weekday() != weekday || localNow.Hour() != hour || localNow.Minute() < minute {
		return time.Time{}, false
	}
	return time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hour, minute, 0, 0, localNow.Location()).UTC(), true
}
