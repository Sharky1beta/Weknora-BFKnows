package handler

import (
	"context"
	"crypto/sha256"
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
	"gorm.io/gorm/clause"

	"github.com/Tencent/WeKnora/internal/datasource/connector/feishu"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

type selfCheckProfile struct {
	TenantID           uint64     `json:"tenant_id" gorm:"primaryKey"`
	KnowledgeBaseID    string     `json:"knowledge_base_id" gorm:"primaryKey;type:varchar(36)"`
	Purpose            string     `json:"purpose" gorm:"type:varchar(24)"`
	SOPKnowledgeBaseID string     `json:"sop_knowledge_base_id" gorm:"type:varchar(36)"`
	SOPVersion         string     `json:"sop_version"`
	OwnerName          string     `json:"owner_name"`
	OwnerEmail         string     `json:"owner_email"`
	OwnerOpenID        string     `json:"owner_open_id"`
	IMChannelID        string     `json:"im_channel_id"`
	DailySchedule      string     `json:"daily_schedule" gorm:"type:varchar(16)"`
	LastRunAt          *time.Time `json:"last_run_at,omitempty"`
	Enabled            bool       `json:"enabled"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (selfCheckProfile) TableName() string { return "knowledge_selfcheck_profiles" }

type selfCheckSOPMapping struct {
	ID                 string    `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID           uint64    `json:"tenant_id"`
	BusinessLine       string    `json:"business_line"`
	ProjectType        string    `json:"project_type"`
	SOPKnowledgeBaseID string    `json:"sop_knowledge_base_id" gorm:"type:varchar(36)"`
	SOPVersion         string    `json:"sop_version"`
	Active             bool      `json:"active"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (selfCheckSOPMapping) TableName() string { return "knowledge_selfcheck_sop_mappings" }

type selfCheckRun struct {
	ID            string     `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID      uint64     `json:"tenant_id"`
	ProjectKBID   string     `json:"project_kb_id" gorm:"column:project_kb_id;type:varchar(36)"`
	ProjectKBName string     `json:"project_kb_name"`
	SOPKBID       string     `json:"sop_kb_id" gorm:"column:sop_kb_id;type:varchar(36)"`
	SOPKBName     string     `json:"sop_kb_name" gorm:"column:sop_kb_name"`
	SOPVersion    string     `json:"sop_version"`
	Status        string     `json:"status" gorm:"type:varchar(24)"`
	Summary       types.JSON `json:"summary" gorm:"type:jsonb"`
	ModelID       string     `json:"model_id"`
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (selfCheckRun) TableName() string { return "knowledge_selfcheck_runs" }

type selfCheckDocumentMapping struct {
	ID                   string    `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID             uint64    `json:"tenant_id"`
	ProjectKBID          string    `json:"project_kb_id" gorm:"type:varchar(36)"`
	ProjectDocumentID    string    `json:"project_document_id" gorm:"type:varchar(36)"`
	ProjectDocumentTitle string    `json:"project_document_title"`
	SOPKBID              string    `json:"sop_kb_id" gorm:"column:sop_kb_id;type:varchar(36)"`
	SOPDocumentID        string    `json:"sop_document_id" gorm:"type:varchar(36)"`
	SOPDocumentTitle     string    `json:"sop_document_title"`
	MatchMethod          string    `json:"match_method"`
	MatchConfidence      float64   `json:"match_confidence"`
	MatchStatus          string    `json:"match_status"`
	ProjectContentHash   string    `json:"project_content_hash"`
	SOPContentHash       string    `json:"sop_content_hash"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (selfCheckDocumentMapping) TableName() string { return "knowledge_selfcheck_document_mappings" }

type selfCheckFinding struct {
	ID                   string    `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID             uint64    `json:"tenant_id"`
	RunID                string    `json:"run_id" gorm:"type:varchar(36)"`
	ProjectDocumentID    string    `json:"project_document_id" gorm:"type:varchar(36)"`
	ProjectDocumentTitle string    `json:"project_document_title"`
	SOPDocumentID        string    `json:"sop_document_id" gorm:"type:varchar(36)"`
	SOPDocumentTitle     string    `json:"sop_document_title"`
	Requirement          string    `json:"requirement"`
	Material             string    `json:"material"`
	State                string    `json:"state" gorm:"type:varchar(24)"`
	Evidence             string    `json:"evidence"`
	SOPClause            string    `json:"sop_clause"`
	Suggestion           string    `json:"suggestion"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// selfCheckSOPDeliverableCache stores the only expensive part of a check: the
// structured deliverables extracted from a complete SOP document.  The cache
// key is the tenant, SOP document and full-content hash, so a project rerun
// never asks the model to re-read an unchanged SOP.
type selfCheckSOPDeliverableCache struct {
	ID             string     `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID       uint64     `json:"tenant_id"`
	SOPKBID        string     `json:"sop_kb_id" gorm:"column:sop_kb_id;type:varchar(36)"`
	SOPDocumentID  string     `json:"sop_document_id" gorm:"type:varchar(36)"`
	SOPContentHash string     `json:"sop_content_hash"`
	PromptVersion  string     `json:"prompt_version"`
	Status         string     `json:"status" gorm:"type:varchar(24)"`
	Summary        string     `json:"summary"`
	Requirements   types.JSON `json:"requirements" gorm:"type:jsonb"`
	Error          string     `json:"error"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (selfCheckSOPDeliverableCache) TableName() string { return "knowledge_selfcheck_sop_deliverables" }

func (selfCheckFinding) TableName() string { return "knowledge_selfcheck_findings" }

type selfCheckOverview struct {
	KnowledgeBases   []types.KnowledgeBase      `json:"knowledge_bases"`
	Profiles         []selfCheckProfile         `json:"profiles"`
	Mappings         []selfCheckSOPMapping      `json:"mappings"`
	DocumentMappings []selfCheckDocumentMapping `json:"document_mappings"`
	Runs             []selfCheckRun             `json:"runs"`
	Findings         []selfCheckFinding         `json:"findings"`
}

type selfCheckFeishuMember struct {
	OpenID string `json:"open_id"`
	Name   string `json:"name"`
}

type updateSelfCheckProfileRequest struct {
	Purpose            string `json:"purpose"`
	SOPKnowledgeBaseID string `json:"sop_knowledge_base_id"`
	SOPVersion         string `json:"sop_version"`
	OwnerName          string `json:"owner_name"`
	OwnerEmail         string `json:"owner_email"`
	OwnerOpenID        string `json:"owner_open_id"`
	IMChannelID        string `json:"im_channel_id"`
	DailySchedule      string `json:"daily_schedule"`
	Enabled            bool   `json:"enabled"`
}

type upsertSelfCheckMappingRequest struct {
	ID                 string `json:"id"`
	BusinessLine       string `json:"business_line"`
	ProjectType        string `json:"project_type"`
	SOPKnowledgeBaseID string `json:"sop_knowledge_base_id"`
	SOPVersion         string `json:"sop_version"`
	Active             bool   `json:"active"`
}

type runSelfCheckRequest struct {
	ProjectKnowledgeBaseID string `json:"project_knowledge_base_id"`
	ModelID                string `json:"model_id"`
}

type selfCheckDocumentMappingRequest struct {
	ProjectKnowledgeBaseID string `json:"project_knowledge_base_id"`
	ProjectDocumentID      string `json:"project_document_id"`
	SOPKnowledgeBaseID     string `json:"sop_knowledge_base_id"`
	SOPDocumentID          string `json:"sop_document_id"`
	Enabled                *bool  `json:"enabled"`
}

type selfCheckDocumentMappingItem struct {
	ProjectDocumentID   string `json:"project_document_id"`
	ProjectDocumentName string `json:"project_document_name"`
	SOPDocumentID       string `json:"sop_document_id"`
	SOPDocumentName     string `json:"sop_document_name"`
	SOPKnowledgeBaseID  string `json:"sop_knowledge_base_id"`
	Enabled             bool   `json:"enabled"`
}
type selfCheckSOPDocumentOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type selfCheckAIResult struct {
	ProjectSummary string               `json:"project_summary"`
	SOPSummary     string               `json:"sop_summary"`
	Findings       []selfCheckAIFinding `json:"findings"`
}

type selfCheckAIFinding struct {
	Requirement string `json:"requirement"`
	Material    string `json:"material"`
	State       string `json:"state"`
	Evidence    string `json:"evidence"`
	SOPClause   string `json:"sop_clause"`
	Suggestion  string `json:"suggestion"`
}

func (h *MessageHandler) GetKnowledgeSelfCheckOverview(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := types.MustTenantIDFromContext(ctx)
	var result selfCheckOverview
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND is_temporary = ?", tenantID, false).
		Order("name").Find(&result.KnowledgeBases).Error; err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	// KnowledgeCount is a computed field (gorm:"-") and therefore is not
	// populated by the knowledge-base query above. Fill it from the knowledge
	// table so the self-check page reports the real document count.
	for i := range result.KnowledgeBases {
		var count int64
		if err := h.db.WithContext(ctx).Model(&types.Knowledge{}).
			Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, result.KnowledgeBases[i].ID).
			Count(&count).Error; err != nil {
			c.Error(errors.NewInternalServerError(err.Error()))
			return
		}
		result.KnowledgeBases[i].KnowledgeCount = count
	}
	kbIDs := make([]string, 0, len(result.KnowledgeBases))
	for _, kb := range result.KnowledgeBases {
		kbIDs = append(kbIDs, kb.ID)
	}
	if len(kbIDs) > 0 {
		_ = h.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_base_id IN ?", tenantID, kbIDs).Order("updated_at DESC").Find(&result.Profiles).Error
		_ = h.db.WithContext(ctx).Where("tenant_id = ? AND project_kb_id IN ?", tenantID, kbIDs).Order("updated_at DESC").Limit(100).Find(&result.DocumentMappings).Error
	}
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("updated_at DESC").Find(&result.Mappings).Error
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("created_at DESC").Limit(30).Find(&result.Runs).Error
	if len(result.Runs) > 0 {
		runIDs := make([]string, 0, len(result.Runs))
		for _, run := range result.Runs {
			runIDs = append(runIDs, run.ID)
		}
		_ = h.db.WithContext(ctx).Where("tenant_id = ? AND run_id IN ?", tenantID, runIDs).
			Order("created_at DESC").Find(&result.Findings).Error
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// ClearKnowledgeSelfCheckRuns removes this tenant's retained audit results.
func (h *MessageHandler) ClearKnowledgeSelfCheckRuns(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := types.MustTenantIDFromContext(ctx)
	if err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ?", tenantID).Delete(&selfCheckFinding{}).Error; err != nil {
			return err
		}
		return tx.Where("tenant_id = ?", tenantID).Delete(&selfCheckRun{}).Error
	}); err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListKnowledgeSelfCheckFeishuMembers exposes a deliberately small contact
// projection for the owner selector. It never exposes credentials or fields
// beyond the display name and app-scoped open_id needed for notifications.
func (h *MessageHandler) ListKnowledgeSelfCheckFeishuMembers(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := types.MustTenantIDFromContext(ctx)
	members, err := h.listSelfCheckFeishuMembers(ctx, tenantID)
	if err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": members})
}

func (h *MessageHandler) UpdateKnowledgeSelfCheckProfile(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := types.MustTenantIDFromContext(ctx)
	var req updateSelfCheckProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	req.Purpose = strings.TrimSpace(req.Purpose)
	if req.Purpose != "project" && req.Purpose != "sop" && req.Purpose != "ignore" {
		c.Error(errors.NewBadRequestError("purpose must be project, sop or ignore"))
		return
	}
	var kb types.KnowledgeBase
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", c.Param("kb_id"), tenantID).First(&kb).Error; err != nil {
		c.Error(errors.NewNotFoundError("knowledge base not found"))
		return
	}
	profile := selfCheckProfile{TenantID: tenantID, KnowledgeBaseID: kb.ID}
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, kb.ID).First(&profile).Error
	profile.Purpose = req.Purpose
	profile.SOPKnowledgeBaseID = strings.TrimSpace(req.SOPKnowledgeBaseID)
	profile.SOPVersion = strings.TrimSpace(req.SOPVersion)
	if req.Purpose == "project" && profile.SOPKnowledgeBaseID != "" {
		var sopKB types.KnowledgeBase
		if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", profile.SOPKnowledgeBaseID, tenantID).First(&sopKB).Error; err != nil {
			c.Error(errors.NewBadRequestError("SOP knowledge base not found"))
			return
		}
	}
	profile.OwnerName = strings.TrimSpace(req.OwnerName)
	profile.OwnerEmail = strings.TrimSpace(req.OwnerEmail)
	profile.OwnerOpenID = strings.TrimSpace(req.OwnerOpenID)
	// Backward-compatible email support: existing API clients can still submit
	// an email. The self-check UI now uses the member selector and submits the
	// app-scoped open_id directly, which works for tenants without email fields.
	if profile.OwnerEmail != "" && profile.OwnerOpenID == "" {
		openID, err := h.resolveSelfCheckOwnerOpenID(ctx, tenantID, profile.OwnerEmail, kb.ID)
		if err != nil {
			c.Error(errors.NewBadRequestError(err.Error()))
			return
		}
		profile.OwnerOpenID = openID
	}
	profile.IMChannelID = strings.TrimSpace(req.IMChannelID)
	dailySchedule, err := normalizeSelfCheckDailySchedule(req.DailySchedule)
	if err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	profile.DailySchedule = dailySchedule
	profile.Enabled = req.Enabled
	if err := h.db.WithContext(ctx).Save(&profile).Error; err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": profile})
}

func (h *MessageHandler) UpsertKnowledgeSelfCheckMapping(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := types.MustTenantIDFromContext(ctx)
	var req upsertSelfCheckMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	if strings.TrimSpace(req.BusinessLine) == "" || strings.TrimSpace(req.ProjectType) == "" || strings.TrimSpace(req.SOPKnowledgeBaseID) == "" {
		c.Error(errors.NewBadRequestError("business_line, project_type and sop_knowledge_base_id are required"))
		return
	}
	var kb types.KnowledgeBase
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", req.SOPKnowledgeBaseID, tenantID).First(&kb).Error; err != nil {
		c.Error(errors.NewBadRequestError("SOP knowledge base not found"))
		return
	}
	if req.Active {
		h.db.WithContext(ctx).Model(&selfCheckSOPMapping{}).Where("tenant_id = ? AND business_line = ? AND project_type = ?", tenantID, strings.TrimSpace(req.BusinessLine), strings.TrimSpace(req.ProjectType)).Update("active", false)
	}
	mapping := selfCheckSOPMapping{ID: req.ID, TenantID: tenantID}
	if mapping.ID == "" {
		mapping.ID = uuid.NewString()
	}
	mapping.BusinessLine = strings.TrimSpace(req.BusinessLine)
	mapping.ProjectType = strings.TrimSpace(req.ProjectType)
	mapping.SOPKnowledgeBaseID = kb.ID
	mapping.SOPVersion = strings.TrimSpace(req.SOPVersion)
	mapping.Active = req.Active
	if err := h.db.WithContext(ctx).Save(&mapping).Error; err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": mapping})
}

func (h *MessageHandler) ListKnowledgeSelfCheckDocumentMappings(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := types.MustTenantIDFromContext(ctx)
	projectKBID := strings.TrimSpace(c.Query("project_knowledge_base_id"))
	if projectKBID == "" {
		c.Error(errors.NewBadRequestError("project_knowledge_base_id is required"))
		return
	}
	var rows []selfCheckDocumentMapping
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND project_kb_id = ?", tenantID, projectKBID).Order("project_document_title").Find(&rows).Error; err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	items := make([]selfCheckDocumentMappingItem, 0, len(rows))
	entryIDs := map[string]bool{}
	var projectDocs []types.Knowledge
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, projectKBID).Order("updated_at DESC").Find(&projectDocs).Error
	entryDocs := h.selfCheckEntryDocuments(projectDocs)
	for _, doc := range entryDocs {
		entryIDs[doc.ID] = true
	}
	for _, row := range rows {
		if !entryIDs[row.ProjectDocumentID] {
			continue
		}
		items = append(items, selfCheckDocumentMappingItem{row.ProjectDocumentID, row.ProjectDocumentTitle, row.SOPDocumentID, row.SOPDocumentTitle, row.SOPKBID, row.MatchStatus != "disabled"})
	}
	known := map[string]bool{}
	for _, item := range items {
		known[item.ProjectDocumentID] = true
	}
	for _, doc := range entryDocs {
		if !known[doc.ID] {
			items = append(items, selfCheckDocumentMappingItem{ProjectDocumentID: doc.ID, ProjectDocumentName: firstNonEmpty(doc.Title, doc.FileName)})
		}
	}
	var sopKBID string
	if profileErr := h.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, projectKBID).Select("sop_knowledge_base_id").Table("knowledge_selfcheck_profiles").Scan(&sopKBID).Error; profileErr == nil && sopKBID != "" {
		var docs []types.Knowledge
		_ = h.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, sopKBID).Order("updated_at DESC").Find(&docs).Error
		options := make([]selfCheckSOPDocumentOption, 0, len(docs))
		for _, doc := range docs {
			options = append(options, selfCheckSOPDocumentOption{doc.ID, firstNonEmpty(doc.Title, doc.FileName)})
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": items, "sop_documents": options}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": items, "sop_documents": []selfCheckSOPDocumentOption{}}})
}

func (h *MessageHandler) UpsertKnowledgeSelfCheckDocumentMapping(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := types.MustTenantIDFromContext(ctx)
	var req selfCheckDocumentMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	req.ProjectKnowledgeBaseID, req.ProjectDocumentID = strings.TrimSpace(req.ProjectKnowledgeBaseID), strings.TrimSpace(req.ProjectDocumentID)
	req.SOPKnowledgeBaseID, req.SOPDocumentID = strings.TrimSpace(req.SOPKnowledgeBaseID), strings.TrimSpace(req.SOPDocumentID)
	if req.ProjectKnowledgeBaseID == "" || req.ProjectDocumentID == "" || req.SOPKnowledgeBaseID == "" || req.SOPDocumentID == "" {
		c.Error(errors.NewBadRequestError("project and SOP document bindings are required"))
		return
	}
	var projectDoc, sopDoc types.Knowledge
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ? AND knowledge_base_id = ?", req.ProjectDocumentID, tenantID, req.ProjectKnowledgeBaseID).First(&projectDoc).Error; err != nil {
		c.Error(errors.NewNotFoundError("project_document_not_found"))
		return
	}
	var projectDocs []types.Knowledge
	_ = h.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, req.ProjectKnowledgeBaseID).Find(&projectDocs).Error
	if !isSelfCheckEntryDocument(projectDoc, projectDocs) {
		c.Error(errors.NewBadRequestError("project_document_is_not_selfcheck_entry"))
		return
	}
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ? AND knowledge_base_id = ?", req.SOPDocumentID, tenantID, req.SOPKnowledgeBaseID).First(&sopDoc).Error; err != nil {
		c.Error(errors.NewNotFoundError("sop_document_not_found"))
		return
	}
	var row selfCheckDocumentMapping
	// A project document has one authoritative SOP binding per tenant. Do not
	// include project_kb_id in the lookup: legacy rows may have been created
	// with a stale KB scope, and that would turn an update into a duplicate.
	err := h.db.WithContext(ctx).Where("tenant_id = ? AND project_document_id = ?", tenantID, req.ProjectDocumentID).Order("created_at ASC").First(&row).Error
	if err == gorm.ErrRecordNotFound {
		row.ID = uuid.NewString()
		row.TenantID = tenantID
		row.ProjectKBID = req.ProjectKnowledgeBaseID
		row.ProjectDocumentID = req.ProjectDocumentID
	}
	row.ProjectDocumentTitle = firstNonEmpty(projectDoc.Title, projectDoc.FileName)
	row.SOPKBID = req.SOPKnowledgeBaseID
	row.SOPDocumentID = req.SOPDocumentID
	row.SOPDocumentTitle = firstNonEmpty(sopDoc.Title, sopDoc.FileName)
	row.MatchMethod = "manual"
	row.MatchConfidence = 1
	row.MatchStatus = "matched"
	if req.Enabled != nil && !*req.Enabled {
		row.MatchStatus = "disabled"
	}
	if err := h.db.WithContext(ctx).Save(&row).Error; err != nil {
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

func (h *MessageHandler) RunKnowledgeSelfCheck(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := types.MustTenantIDFromContext(ctx)
	var req runSelfCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	projectKBID := strings.TrimSpace(req.ProjectKnowledgeBaseID)
	modelID := strings.TrimSpace(req.ModelID)
	// Do not hold the browser/proxy connection open for the three model calls.
	// The job keeps the tenant in a detached context and persists its result;
	// the page can refresh the overview while it is running.
	jobCtx := context.WithValue(context.Background(), types.TenantIDContextKey, tenantID)
	go func() {
		_, _, _ = h.runKnowledgeSelfCheck(jobCtx, tenantID, projectKBID, modelID)
	}()
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": gin.H{"status": "running", "project_kb_id": projectKBID}})
}

func (h *MessageHandler) runKnowledgeSelfCheck(ctx context.Context, tenantID uint64, projectKBID, modelID string) (*selfCheckRun, []selfCheckFinding, error) {
	var profile selfCheckProfile
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_base_id = ? AND purpose = ? AND enabled = ?", tenantID, projectKBID, "project", true).First(&profile).Error; err != nil {
		return nil, nil, fmt.Errorf("该知识库未启用知识自检")
	}
	var projectKB types.KnowledgeBase
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", projectKBID, tenantID).First(&projectKB).Error; err != nil {
		return nil, nil, fmt.Errorf("项目知识库不存在")
	}
	if strings.TrimSpace(profile.SOPKnowledgeBaseID) == "" {
		return nil, nil, fmt.Errorf("项目知识库尚未配置 SOP 知识库")
	}
	var sopKB types.KnowledgeBase
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", profile.SOPKnowledgeBaseID, tenantID).First(&sopKB).Error; err != nil {
		return nil, nil, fmt.Errorf("配置的 SOP 知识库不存在")
	}

	var projectDocs []types.Knowledge
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_base_id = ? AND parse_status = ?", tenantID, projectKB.ID, types.ParseStatusCompleted).Order("updated_at DESC").Find(&projectDocs).Error; err != nil || len(projectDocs) == 0 {
		return nil, nil, fmt.Errorf("项目知识库中没有可读取的项目文档")
	}
	var sopDocs []types.Knowledge
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_base_id = ? AND parse_status = ?", tenantID, sopKB.ID, types.ParseStatusCompleted).Order("updated_at DESC").Find(&sopDocs).Error; err != nil || len(sopDocs) == 0 {
		return nil, nil, fmt.Errorf("SOP 知识库中没有可读取的 SOP 文档")
	}

	tenantCtx := context.WithValue(ctx, types.TenantIDContextKey, tenantID)
	analysisCtx, cancel := context.WithTimeout(tenantCtx, 300*time.Second)
	defer cancel()
	model, resolvedModelID, err := h.resolveQualityReportModel(tenantCtx, modelID)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	run := &selfCheckRun{ID: uuid.NewString(), TenantID: tenantID, ProjectKBID: projectKB.ID, ProjectKBName: projectKB.Name, SOPKBID: sopKB.ID, SOPKBName: sopKB.Name, SOPVersion: profile.SOPVersion, Status: "running", Summary: types.JSON([]byte(`{}`)), ModelID: resolvedModelID, StartedAt: now}
	if err := h.db.WithContext(ctx).Create(run).Error; err != nil {
		return nil, nil, err
	}
	failRun := func(cause error) (*selfCheckRun, []selfCheckFinding, error) {
		finished := time.Now().UTC()
		body, _ := json.Marshal(map[string]any{"error": cause.Error()})
		run.Status, run.Summary, run.CompletedAt = "failed", types.JSON(body), &finished
		_ = h.db.WithContext(ctx).Save(run).Error
		return run, nil, cause
	}

	entryDocs := h.selfCheckEntryDocuments(projectDocs)
	if len(entryDocs) == 0 {
		return failRun(fmt.Errorf("未识别到入口项目文档，请确认项目知识库中存在 PRD、项目方案、交付稿等入口文档"))
	}

	allFindings := make([]selfCheckFinding, 0)
	docSummaries := make([]map[string]any, 0)
	for _, pDoc := range entryDocs {
		pTitle := firstNonEmpty(pDoc.Title, pDoc.FileName)
		pText, pHash, err := h.singleDocumentTextAndHash(ctx, tenantID, pDoc.ID, 0)
		if err != nil || strings.TrimSpace(pText) == "" {
			docSummaries = append(docSummaries, map[string]any{"project_doc_id": pDoc.ID, "project_doc_title": pTitle, "status": "document_unreadable", "findings_count": 0})
			continue
		}

		var existing selfCheckDocumentMapping
		boundErr := h.db.WithContext(ctx).Where("tenant_id = ? AND project_document_id = ?", tenantID, pDoc.ID).Order("updated_at DESC").First(&existing).Error
		var sDoc *types.Knowledge
		matchMethod, confidence := "binding", 1.0
		if boundErr == nil && existing.SOPDocumentID != "" {
			for i := range sopDocs {
				if sopDocs[i].ID == existing.SOPDocumentID {
					sDoc = &sopDocs[i]
					break
				}
			}
		} else {
			sDoc, matchMethod, confidence = h.matchSOPDocument(pTitle, pText, sopDocs)
		}
		if sDoc == nil {
			allFindings = append(allFindings, selfCheckFinding{ID: uuid.NewString(), TenantID: tenantID, RunID: run.ID, ProjectDocumentID: pDoc.ID, ProjectDocumentTitle: pTitle, Requirement: "未匹配到 SOP", State: "unmatched", Evidence: "未找到该入口项目文档对应的 SOP", Suggestion: "请在入口文档绑定区手动选择对应 SOP"})
			docSummaries = append(docSummaries, map[string]any{"project_doc_id": pDoc.ID, "project_doc_title": pTitle, "status": "unmatched", "findings_count": 1})
			continue
		}

		sTitle := firstNonEmpty(sDoc.Title, sDoc.FileName)
		sText, sHash, err := h.singleDocumentTextAndHash(ctx, tenantID, sDoc.ID, 0)
		if err != nil || strings.TrimSpace(sText) == "" {
			allFindings = append(allFindings, selfCheckFinding{ID: uuid.NewString(), TenantID: tenantID, RunID: run.ID, ProjectDocumentID: pDoc.ID, ProjectDocumentTitle: pTitle, SOPDocumentID: sDoc.ID, SOPDocumentTitle: sTitle, Requirement: "无法读取 SOP", State: "sop_unreadable", Evidence: "SOP 文档未读取到有效 chunk", SOPClause: sTitle, Suggestion: "请确认 SOP 文档已完成解析"})
			docSummaries = append(docSummaries, map[string]any{"project_doc_id": pDoc.ID, "project_doc_title": pTitle, "sop_doc_title": sTitle, "status": "sop_unreadable", "findings_count": 1})
			continue
		}

		mapping := selfCheckDocumentMapping{ID: uuid.NewString(), TenantID: tenantID, ProjectKBID: projectKB.ID, ProjectDocumentID: pDoc.ID, ProjectDocumentTitle: pTitle, SOPKBID: sopKB.ID, SOPDocumentID: sDoc.ID, SOPDocumentTitle: sTitle, MatchMethod: matchMethod, MatchConfidence: confidence, MatchStatus: "matched", ProjectContentHash: pHash, SOPContentHash: sHash}
		if existing.ID != "" {
			mapping.ID = existing.ID
		}
		_ = h.db.WithContext(ctx).Save(&mapping).Error

		reqs, sopSummary, err := h.cachedSelfCheckSOPRequirements(analysisCtx, tenantID, sopKB.ID, sDoc.ID, sTitle, sText, sHash, model)
		_ = sopSummary
		if err != nil || len(reqs) == 0 {
			errText := "SOP 交付物提取失败，未得到可检查的实体交付物"
			if err != nil {
				errText = "SOP 交付物提取失败：" + err.Error()
			}
			allFindings = append(allFindings, selfCheckFinding{ID: uuid.NewString(), TenantID: tenantID, RunID: run.ID, ProjectDocumentID: pDoc.ID, ProjectDocumentTitle: pTitle, SOPDocumentID: sDoc.ID, SOPDocumentTitle: sTitle, Requirement: "SOP 交付物清单", State: "sop_unreadable", Evidence: errText, SOPClause: sTitle, Suggestion: "请确认模型服务可用后重试 SOP 交付物提取"})
			docSummaries = append(docSummaries, map[string]any{"project_doc_id": pDoc.ID, "project_doc_title": pTitle, "sop_doc_title": sTitle, "status": "no_sop_requirements", "findings_count": 1, "error": errText})
			continue
		}

		scopeDocs := h.selfCheckScopeDocuments(pDoc, projectDocs)
		evidence := make([]selfCheckEvidenceDocument, 0, len(scopeDocs))
		for _, doc := range scopeDocs {
			text, _, _ := h.singleDocumentTextAndHash(ctx, tenantID, doc.ID, 0)
			evidence = append(evidence, selfCheckEvidenceDocument{ID: doc.ID, Title: firstNonEmpty(doc.Title, doc.FileName), Headings: extractSelfCheckHeadings(text)})
		}

		docFindingsCount := 0
		docStatus := "present"
		for _, req := range reqs {
			name := strings.TrimSpace(req.Requirement)
			if name == "" {
				continue
			}
			state, matchedBy, matchedDoc, matchedHeading := matchSelfCheckArtifact(name, evidence)
			displayRequirement := name
			if state == "partial" {
				if missingTerms := missingSelfCheckArtifactTerms(name, evidence); len(missingTerms) > 0 {
					displayRequirement = strings.Join(missingTerms, "、")
				}
			}
			if state == "missing" && docStatus == "present" {
				docStatus = "missing"
			}
			if state == "uncertain" && docStatus == "present" {
				docStatus = "uncertain"
			}
			if state == "partial" && (docStatus == "present" || docStatus == "uncertain") {
				docStatus = "partial"
			}
			material := ""
			if matchedDoc.Title != "" {
				material = matchedDoc.Title
			}
			evidenceText := "未找到明确证据"
			if state == "present" || state == "uncertain" {
				evidenceText = fmt.Sprintf("%s：%s", matchedBy, matchedDoc.Title)
				if matchedHeading != "" {
					evidenceText += " / " + matchedHeading
				}
			}
			allFindings = append(allFindings, selfCheckFinding{ID: uuid.NewString(), TenantID: tenantID, RunID: run.ID, ProjectDocumentID: pDoc.ID, ProjectDocumentTitle: pTitle, SOPDocumentID: sDoc.ID, SOPDocumentTitle: sTitle, Requirement: displayRequirement, Material: material, State: state, Evidence: evidenceText, SOPClause: req.SOPClause, Suggestion: selfCheckSuggestion(state)})
			docFindingsCount++
		}
		if docFindingsCount == 0 {
			docStatus = "missing"
		}
		docSummaries = append(docSummaries, map[string]any{"project_doc_id": pDoc.ID, "project_doc_title": pTitle, "sop_doc_title": sTitle, "status": docStatus, "findings_count": docFindingsCount, "scope_docs": len(scopeDocs)})
	}

	if len(docSummaries) == 0 {
		return failRun(fmt.Errorf("本次检查未产生明细：未找到可核对的交付要求或模型未返回结果"))
	}
	rawSummary, _ := json.Marshal(map[string]any{"doc_summaries": docSummaries, "total_entry_docs": len(entryDocs), "total_findings": len(allFindings)})
	finished := time.Now().UTC()
	completedDocs, failedDocs := 0, 0
	for _, summary := range docSummaries {
		status, _ := summary["status"].(string)
		switch status {
		case "present", "partial", "missing", "uncertain":
			completedDocs++
		default:
			failedDocs++
		}
	}
	runStatus := "completed"
	if completedDocs == 0 {
		runStatus = "failed"
	} else if failedDocs > 0 {
		runStatus = "partial_failed"
	}
	run.Status, run.Summary, run.CompletedAt = runStatus, types.JSON(rawSummary), &finished
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(run).Error; err != nil {
			return err
		}
		if len(allFindings) > 0 {
			return tx.Create(&allFindings).Error
		}
		return nil
	})
	if err == nil {
		h.notifySelfCheckOwner(ctx, profile, projectKB.Name, sopKB.Name, allFindings)
	}
	return run, allFindings, err
}

type selfCheckEvidenceDocument struct {
	ID       string
	Title    string
	Headings []string
}

func (h *MessageHandler) selfCheckEntryDocuments(docs []types.Knowledge) []types.Knowledge {
	var entries []types.Knowledge
	for _, doc := range docs {
		if isSelfCheckEntryDocument(doc, docs) {
			entries = append(entries, doc)
		}
	}
	if len(entries) == 0 && len(docs) == 1 {
		return docs
	}
	return entries
}

func isSelfCheckEntryDocument(doc types.Knowledge, allDocs []types.Knowledge) bool {
	title := strings.ToLower(firstNonEmpty(doc.Title, doc.FileName))
	if containsAny(title, []string{"prd", "产品方案", "方案设计", "项目方案", "需求说明", "项目启动", "项目规划", "服务交接", "服务方案", "运营方案", "发布方案", "部署方案"}) {
		return true
	}
	if len(allDocs) == 1 {
		return true
	}
	return false
}

func (h *MessageHandler) selfCheckScopeDocuments(entry types.Knowledge, docs []types.Knowledge) []types.Knowledge {
	key := inferSelfCheckScopeKey(firstNonEmpty(entry.Title, entry.FileName))
	var scoped []types.Knowledge
	for _, doc := range docs {
		title := firstNonEmpty(doc.Title, doc.FileName)
		if doc.ID == entry.ID || (key != "" && strings.Contains(normalizeSelfCheckText(title), normalizeSelfCheckText(key))) {
			scoped = append(scoped, doc)
		}
	}
	if len(scoped) == 0 {
		scoped = append(scoped, entry)
	}
	return scoped
}

func inferSelfCheckScopeKey(title string) string {
	base := strings.TrimSuffix(strings.TrimSpace(title), ".md")
	for _, sep := range []string{"_PRD", "-PRD", " PRD", "_产品方案", "-产品方案", "_方案", "-方案", "_需求", "-需求"} {
		if idx := strings.Index(strings.ToLower(base), strings.ToLower(sep)); idx > 0 {
			return strings.Trim(base[:idx], " _-")
		}
	}
	parts := strings.FieldsFunc(base, func(r rune) bool { return r == '_' || r == '-' || r == ' ' })
	if len(parts) >= 2 {
		return strings.Join(parts[:2], "_")
	}
	return base
}

func extractSelfCheckHeadings(text string) []string {
	var headings []string
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			heading := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			if heading != "" {
				headings = append(headings, heading)
			}
			continue
		}
		if len([]rune(trimmed)) <= 40 && (strings.HasSuffix(trimmed, "：") || strings.HasSuffix(trimmed, ":")) {
			headings = append(headings, strings.TrimRight(trimmed, "：:"))
		}
	}
	return headings
}

func matchSelfCheckArtifact(name string, docs []selfCheckEvidenceDocument) (string, string, selfCheckEvidenceDocument, string) {
	target := normalizeSelfCheckText(name)
	if target == "" {
		return "uncertain", "", selfCheckEvidenceDocument{}, ""
	}
	requiresStrongEvidence := isSelfCheckStrongEvidenceArtifact(name)
	if terms := selfCheckArtifactMatchTerms(name); len(terms) > 1 {
		matched := make([]string, 0, len(terms))
		var firstDoc selfCheckEvidenceDocument
		for _, term := range terms {
			state, _, doc, heading := matchSelfCheckArtifactTerm(term, docs, false)
			if state == "present" {
				if firstDoc.Title == "" {
					firstDoc = doc
				}
				if heading != "" {
					matched = append(matched, heading)
				} else if doc.Title != "" {
					matched = append(matched, doc.Title)
				} else {
					matched = append(matched, term)
				}
			}
		}
		if len(matched) == len(terms) {
			return "present", "目录/标题组合", firstDoc, strings.Join(matched, "；")
		}
		if len(matched) > 0 {
			return "partial", "目录/标题部分命中", firstDoc, strings.Join(matched, "；")
		}
	}
	for _, doc := range docs {
		if strings.Contains(normalizeSelfCheckText(doc.Title), target) {
			return "present", "文档标题", doc, ""
		}
	}
	for _, doc := range docs {
		for _, heading := range doc.Headings {
			if strings.Contains(normalizeSelfCheckText(heading), target) || strings.Contains(target, normalizeSelfCheckText(heading)) {
				return "present", "目录章节", doc, heading
			}
		}
	}
	for _, doc := range docs {
		for _, heading := range doc.Headings {
			if requiresStrongEvidence {
				continue
			}
			if weakSelfCheckMatch(target, normalizeSelfCheckText(heading)) {
				return "uncertain", "弱目录章节", doc, heading
			}
		}
	}
	return "missing", "", selfCheckEvidenceDocument{}, ""
}

func normalizeSelfCheckText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("\u300a", "", "\u300b", "", "\uff08", "", "\uff09", "", "\uff1a", "", "\uff0c", "", "\u3001", "", "\uff1b", "", "\u002b", "").Replace(value)
	replacer := strings.NewReplacer(".md", "", ".docx", "", ".xlsx", "", ".png", "", ".jpg", "", ".jpeg", "", " ", "", "_", "", "-", "", "。", "", "，", "", "（", "", "）", "", "(", "", ")", "")
	return replacer.Replace(value)
}

func matchSelfCheckArtifactTerm(name string, docs []selfCheckEvidenceDocument, allowWeak bool) (string, string, selfCheckEvidenceDocument, string) {
	target := normalizeSelfCheckText(name)
	if target == "" {
		return "missing", "", selfCheckEvidenceDocument{}, ""
	}
	targets := selfCheckArtifactAliases(name)
	for _, doc := range docs {
		normalizedTitle := normalizeSelfCheckText(doc.Title)
		for _, alias := range targets {
			if strings.Contains(normalizedTitle, alias) {
				return "present", "文档标题", doc, ""
			}
		}
	}
	for _, doc := range docs {
		for _, heading := range doc.Headings {
			normalizedHeading := normalizeSelfCheckText(heading)
			for _, alias := range targets {
				if strings.Contains(normalizedHeading, alias) || strings.Contains(alias, normalizedHeading) {
					return "present", "目录章节", doc, heading
				}
			}
		}
	}
	if allowWeak {
		for _, doc := range docs {
			for _, heading := range doc.Headings {
				normalizedHeading := normalizeSelfCheckText(heading)
				for _, alias := range targets {
					if weakSelfCheckMatch(alias, normalizedHeading) {
						return "uncertain", "弱目录章节", doc, heading
					}
				}
			}
		}
	}
	return "missing", "", selfCheckEvidenceDocument{}, ""
}

func missingSelfCheckArtifactTerms(name string, docs []selfCheckEvidenceDocument) []string {
	terms := selfCheckArtifactMatchTerms(name)
	if len(terms) <= 1 {
		return nil
	}
	missing := make([]string, 0, len(terms))
	for _, term := range terms {
		state, _, _, _ := matchSelfCheckArtifactTerm(term, docs, false)
		if state != "present" {
			missing = append(missing, term)
		}
	}
	return missing
}

func selfCheckArtifactMatchTerms(name string) []string {
	raw := strings.TrimSpace(name)
	if raw == "" {
		return nil
	}
	terms := []string{raw}
	if start := strings.Index(raw, "\u5305\u542b"); start >= 0 {
		prefix := strings.TrimSpace(raw[:start])
		prefix = strings.Trim(prefix, "\u300a\u300b\uff08\uff09() \t")
		terms = []string{}
		children := raw[start+len("\u5305\u542b"):]
		children = strings.Trim(children, "?:??() \t")
		for _, part := range strings.FieldsFunc(children, func(r rune) bool {
			return r == '\u3001' || r == '\uff0c' || r == ',' || r == '+' || r == '/' || r == '\uff1b' || r == ';'
		}) {
			part = strings.TrimSpace(strings.Trim(part, "\u300a\u300b\uff08\uff09() \t"))
			if part != "" {
				terms = append(terms, part)
			}
		}
		if len(terms) == 0 && prefix != "" {
			terms = append(terms, prefix)
		}
	}
	seen := map[string]bool{}
	clean := make([]string, 0, len(terms))
	for _, term := range terms {
		key := normalizeSelfCheckText(term)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		clean = append(clean, term)
	}
	return clean
}

func selfCheckArtifactAliases(name string) []string {
	base := normalizeSelfCheckText(name)
	aliases := []string{base}
	aliasMap := map[string][]string{
		"\u4ea7\u54c1\u89e3\u51b3\u65b9\u6848\u6587\u6863": {"\u4ea7\u54c1\u89e3\u51b3\u65b9\u6848", "\u89e3\u51b3\u65b9\u6848", "\u4ea7\u54c1\u65b9\u6848", "\u65b9\u6848\u6587\u6863"},
		"\u89e3\u51b3\u65b9\u6848\u6587\u6863":             {"\u4ea7\u54c1\u89e3\u51b3\u65b9\u6848", "\u89e3\u51b3\u65b9\u6848", "\u4ea7\u54c1\u65b9\u6848", "\u65b9\u6848\u6587\u6863"},
		"\u529f\u80fd\u6e05\u5355\u4e0e\u4f18\u5148\u7ea7": {"\u529f\u80fd\u6e05\u5355", "\u4f18\u5148\u7ea7", "\u4f18\u5148\u7ea7\u5b9a\u4e49", "moscow"},
		"\u67b6\u6784\u56fe":                               {"\u67b6\u6784\u56fe", "\u4fe1\u606f\u67b6\u6784\u56fe", "\u7cfb\u7edf\u67b6\u6784\u56fe", "\u4e1a\u52a1\u6d41\u7a0b\u56fe"},
		"\u4fe1\u606f\u67b6\u6784\u56fe":                   {"\u67b6\u6784\u56fe", "\u4fe1\u606f\u67b6\u6784\u56fe", "\u7cfb\u7edf\u67b6\u6784\u56fe", "\u4e1a\u52a1\u6d41\u7a0b\u56fe"},
		"prd\u5b8c\u6574\u7a3f":                            {"prd", "\u9700\u6c42\u89c4\u683c", "\u9700\u6c42\u89c4\u683c\u8bf4\u660e", "\u529f\u80fd\u8bf4\u660e", "\u9a8c\u6536\u6807\u51c6"},
		"\u9ad8\u4fdd\u771f\u4ea4\u4e92\u539f\u578b":       {"\u539f\u578b", "\u4ea4\u4e92\u539f\u578b", "\u9ad8\u4fdd\u771f\u539f\u578b", "\u9ad8\u4fdd\u771f\u4ea4\u4e92\u539f\u578b", "\u539f\u578b\u4e0e\u8d70\u67e5\u8bb0\u5f55"},
		"\u7528\u6237\u753b\u50cf\u5361\u7247":             {"\u7528\u6237\u753b\u50cf", "\u7528\u6237\u753b\u50cf\u5361\u7247"},
		"\u7528\u6237\u65c5\u7a0b\u5730\u56fe":             {"\u7528\u6237\u65c5\u7a0b", "\u7528\u6237\u65c5\u7a0b\u5730\u56fe", "journeymap"},
		"storyboard":                                       {"storyboard", "storyboard\u7528\u6237\u6545\u4e8b\u753b\u677f", "\u7528\u6237\u6545\u4e8b\u753b\u677f"},
		"storyboard\u7528\u6237\u6545\u4e8b\u753b\u677f":   {"storyboard", "storyboard\u7528\u6237\u6545\u4e8b\u753b\u677f", "\u7528\u6237\u6545\u4e8b\u753b\u677f"},
	}
	for key, values := range aliasMap {
		normalizedKey := normalizeSelfCheckText(key)
		if base == normalizedKey || strings.Contains(base, normalizedKey) || strings.Contains(normalizedKey, base) {
			for _, value := range values {
				aliases = append(aliases, normalizeSelfCheckText(value))
			}
		}
	}
	seen := map[string]bool{}
	clean := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		if alias == "" || seen[alias] {
			continue
		}
		seen[alias] = true
		clean = append(clean, alias)
	}
	return clean
}

func isSelfCheckStrongEvidenceArtifact(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	strongWords := []string{"ppt", "\u6f14\u793a\u7a3f", "\u5e7b\u706f\u7247", "deck", "\u4f1a\u8bae\u7eaa\u8981", "\u7533\u8bf7\u8868", "\u8ba1\u5212\u8868"}
	return containsAny(lower, strongWords)
}

func weakSelfCheckMatch(a, b string) bool {
	if len([]rune(a)) < 3 || len([]rune(b)) < 3 {
		return false
	}
	common := 0
	seen := map[rune]bool{}
	for _, r := range a {
		seen[r] = true
	}
	for _, r := range b {
		if seen[r] {
			common++
		}
	}
	return common >= 3 && common*2 >= len([]rune(a))
}

func selfCheckSuggestion(state string) string {
	switch state {
	case "present":
		return ""
	case "uncertain":
		return "请确认该弱匹配是否可作为归档交付物证据"
	default:
		return "请在该项目材料范围内补充对应文档或目录章节"
	}
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func (h *MessageHandler) resolveSelfCheckOwnerOpenID(ctx context.Context, tenantID uint64, email, _ string) (string, error) {
	var sources []types.DataSource
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND type = ? AND status <> ?", tenantID, types.ConnectorTypeFeishu, types.DataSourceStatusDeleted).
		Order("CASE WHEN status = 'active' THEN 0 ELSE 1 END, updated_at DESC").
		Find(&sources).Error; err != nil {
		return "", fmt.Errorf("load Feishu data sources: %w", err)
	}
	if len(sources) == 0 {
		return "", fmt.Errorf("no Feishu data source is available to resolve the owner email")
	}

	var lastErr error
	for _, source := range sources {
		config, err := source.ParseConfig()
		if err != nil || config == nil {
			lastErr = err
			continue
		}
		appID, _ := config.Credentials["app_id"].(string)
		appSecret, _ := config.Credentials["app_secret"].(string)
		baseURL, _ := config.Credentials["base_url"].(string)
		if appID == "" || appSecret == "" {
			continue
		}
		client := feishu.NewClient(&feishu.Config{AppID: appID, AppSecret: appSecret, BaseURL: baseURL})
		openID, err := client.ResolveUserOpenIDByEmail(ctx, email)
		if err == nil {
			return openID, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", fmt.Errorf("unable to resolve owner email %s: %w", email, lastErr)
	}
	return "", fmt.Errorf("no Feishu data source with usable credentials is available to resolve %s", email)
}

func (h *MessageHandler) listSelfCheckFeishuMembers(ctx context.Context, tenantID uint64) ([]selfCheckFeishuMember, error) {
	var sources []types.DataSource
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND type = ? AND status <> ?", tenantID, types.ConnectorTypeFeishu, types.DataSourceStatusDeleted).
		Order("CASE WHEN status = 'active' THEN 0 ELSE 1 END, updated_at DESC").
		Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("load Feishu data sources: %w", err)
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("no Feishu data source is available to load members")
	}

	var lastErr error
	for _, source := range sources {
		config, err := source.ParseConfig()
		if err != nil || config == nil {
			lastErr = err
			continue
		}
		appID, _ := config.Credentials["app_id"].(string)
		appSecret, _ := config.Credentials["app_secret"].(string)
		baseURL, _ := config.Credentials["base_url"].(string)
		if appID == "" || appSecret == "" {
			continue
		}
		users, err := feishu.NewClient(&feishu.Config{AppID: appID, AppSecret: appSecret, BaseURL: baseURL}).ListTenantUsers(ctx)
		if err != nil {
			lastErr = err
			continue
		}
		members := make([]selfCheckFeishuMember, 0, len(users))
		for _, user := range users {
			members = append(members, selfCheckFeishuMember{OpenID: user.OpenID, Name: user.Name})
		}
		sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })
		return members, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("unable to load Feishu members: %w", lastErr)
	}
	return nil, fmt.Errorf("no Feishu data source with usable credentials is available to load members")
}

func (h *MessageHandler) notifySelfCheckOwner(ctx context.Context, profile selfCheckProfile, projectName, sopName string, findings []selfCheckFinding) {
	if h.imService == nil || strings.TrimSpace(profile.IMChannelID) == "" || strings.TrimSpace(profile.OwnerOpenID) == "" {
		return
	}
	issues := make([]selfCheckFinding, 0)
	for _, finding := range findings {
		if finding.State != "present" && finding.State != "completed" {
			issues = append(issues, finding)
		}
	}
	sort.SliceStable(issues, func(i, j int) bool {
		leftDoc := firstNonEmpty(issues[i].ProjectDocumentTitle, issues[i].ProjectDocumentID)
		rightDoc := firstNonEmpty(issues[j].ProjectDocumentTitle, issues[j].ProjectDocumentID)
		if leftDoc == rightDoc {
			return issues[i].Requirement < issues[j].Requirement
		}
		return leftDoc < rightDoc
	})
	var body strings.Builder
	if len(issues) == 0 {
		fmt.Fprintf(&body, "知识库自检完成，本次无待处理项\n项目：%s\n依据 SOP：%s", projectName, sopName)
	} else {
		fmt.Fprintf(&body, "知识库自检发现 %d 项需要处理\n项目：%s\n依据 SOP：%s\n", len(issues), projectName, sopName)
	}
	currentDoc := ""
	written := 0
	for _, item := range issues {
		if written >= 12 {
			fmt.Fprintf(&body, "\n…另有 %d 项，请进入 WeKnora「知识自检」查看。", len(issues)-written)
			break
		}
		docTitle := firstNonEmpty(item.ProjectDocumentTitle, item.ProjectDocumentID, "未命名文档")
		if docTitle != currentDoc {
			currentDoc = docTitle
			fmt.Fprintf(&body, "\n\n文档：%s", docTitle)
		}
		written++
		fmt.Fprintf(&body, "\n%d. [%s] %s", written, item.State, item.Requirement)
		if strings.TrimSpace(item.Suggestion) != "" {
			fmt.Fprintf(&body, "\n   建议：%s", item.Suggestion)
		}
	}
	content := body.String()
	if sendErr := h.imService.SendProactiveUserMessage(ctx, profile.IMChannelID, profile.OwnerOpenID, content); sendErr != nil {
		logger.Errorf(ctx, "[KnowledgeSelfCheck] notify user failed channel=%s owner=%s error=%v", profile.IMChannelID, profile.OwnerOpenID, sendErr)
	} else {
		logger.Infof(ctx, "[KnowledgeSelfCheck] notify user sent channel=%s owner=%s issues=%d", profile.IMChannelID, profile.OwnerOpenID, len(issues))
	}
}

func (h *MessageHandler) singleDocumentTextAndHash(ctx context.Context, tenantID uint64, docID string, maxBytes int) (string, string, error) {
	var chunks []types.Chunk
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ? AND is_enabled = ?", tenantID, docID, true).Order("chunk_index").Find(&chunks).Error; err != nil {
		return "", "", err
	}
	var b strings.Builder
	for _, chunk := range chunks {
		if strings.TrimSpace(chunk.Content) != "" {
			b.WriteString(chunk.Content)
			b.WriteString("\n")
		}
	}
	text := b.String()
	// maxBytes is retained for API compatibility, but full-document reads are
	// required for SOP extraction. Callers that need a preview may truncate
	// after hashing; never hash a truncated SOP.
	if maxBytes > 0 && len(text) > maxBytes {
		text = text[:maxBytes]
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
	return text, hash, nil
}

func (h *MessageHandler) matchSOPDocument(pTitle, pText string, sopDocs []types.Knowledge) (*types.Knowledge, string, float64) {
	if len(sopDocs) == 0 {
		return nil, "unmatched", 0.0
	}
	pLower := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(pTitle), ".md"))
	bodyLower := strings.ToLower(pText)
	// Prefer explicit references first. This preserves the old successful match
	// when the project material names its SOP, without relying on list order.
	for i := range sopDocs {
		sTitle := strings.ToLower(strings.TrimSuffix(firstNonEmpty(sopDocs[i].Title, sopDocs[i].FileName), ".md"))
		if sTitle != "" && ((pLower != "" && (strings.Contains(pLower, sTitle) || strings.Contains(sTitle, pLower))) || strings.Contains(bodyLower, sTitle)) {
			method := "title"
			if !strings.Contains(pLower, sTitle) && !strings.Contains(sTitle, pLower) {
				method = "body_title"
			}
			return &sopDocs[i], method, 0.95
		}
	}
	// Deterministic business aliases cover common naming conventions (for
	// example RSH_*_PRD -> 产品方案设计 SOP) while rejecting unrelated SOPs such
	// as 测试设计 and 开发 SOP. Ties remain unmatched instead of guessing.
	markers := []string{}
	if strings.Contains(pLower, "prd") || strings.Contains(pLower, "方案") || strings.Contains(pLower, "需求") {
		markers = append(markers, "方案", "产品")
	}
	if strings.Contains(pLower, "测试") || strings.Contains(pLower, "test") {
		markers = append(markers, "测试")
	}
	if strings.Contains(pLower, "开发") || strings.Contains(pLower, "code") || strings.Contains(pLower, "cpp") {
		markers = append(markers, "开发")
	}
	best, bestScore, tie := -1, 0, false
	for i := range sopDocs {
		sTitle := strings.ToLower(firstNonEmpty(sopDocs[i].Title, sopDocs[i].FileName))
		score := 0
		for _, marker := range markers {
			if strings.Contains(sTitle, marker) {
				score++
			}
		}
		if score > bestScore {
			best, bestScore, tie = i, score, false
		} else if score > 0 && score == bestScore {
			tie = true
		}
	}
	if best >= 0 && bestScore > 0 && !tie {
		return &sopDocs[best], "title_alias", 0.80
	}
	return nil, "unmatched", 0.0
}

func (h *MessageHandler) selfCheckKnowledgeText(ctx context.Context, tenantID uint64, kbID string, maxBytes, maxDocs int, preferChecklistDocs bool) (string, error) {
	var docs []types.Knowledge
	query := h.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_base_id = ? AND parse_status = ?", tenantID, kbID, types.ParseStatusCompleted)
	if preferChecklistDocs {
		// A SOP library name has already been selected manually. Within that
		// library, give the limited reading budget to documents most likely to
		// contain an actionable delivery checklist, instead of scanning every SOP.
		query = query.Order("CASE WHEN COALESCE(title, '') ILIKE '%SOP%' OR COALESCE(file_name, '') ILIKE '%SOP%' THEN 0 WHEN COALESCE(title, '') ILIKE '%娴溿倓绮?' OR COALESCE(file_name, '') ILIKE '%娴溿倓绮?' THEN 1 WHEN COALESCE(title, '') ILIKE '%濞撳懎宕?' OR COALESCE(file_name, '') ILIKE '%濞撳懎宕?' THEN 2 ELSE 3 END").Order("updated_at DESC")
	} else {
		query = query.Order("updated_at DESC")
	}
	if err := query.Find(&docs).Error; err != nil {
		return "", err
	}
	var b strings.Builder
	for i, doc := range docs {
		if maxDocs > 0 && i >= maxDocs {
			break
		}
		b.WriteString("\n\n# 文档：")
		b.WriteString(firstNonEmpty(doc.Title, doc.FileName))
		b.WriteString("\n")
		var chunks []types.Chunk
		if err := h.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ? AND is_enabled = ?", tenantID, doc.ID, true).Order("chunk_index").Limit(120).Find(&chunks).Error; err != nil {
			return "", err
		}
		for _, chunk := range chunks {
			if strings.TrimSpace(chunk.Content) != "" {
				b.WriteString(chunk.Content)
				b.WriteString("\n")
			}
		}
		if b.Len() >= maxBytes {
			break
		}
	}
	text := b.String()
	if len(text) > maxBytes {
		text = text[:maxBytes]
	}
	return text, nil
}

type selfCheckSOPRequirement struct {
	Requirement string `json:"requirement"`
	SOPClause   string `json:"sop_clause"`
	Condition   string `json:"condition"`
}

type selfCheckSOPExtractionPayload struct {
	Summary      string                    `json:"summary"`
	Requirements []selfCheckSOPRequirement `json:"requirements"`
}

const selfCheckSOPPromptVersion = "entity-artifacts-v9"

func (h *MessageHandler) cachedSelfCheckSOPRequirements(ctx context.Context, tenantID uint64, sopKBID, sopDocID, sopTitle, sopText, contentHash string, model chat.Chat) ([]selfCheckSOPRequirement, string, error) {
	var cached selfCheckSOPDeliverableCache
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND sop_kb_id = ? AND sop_document_id = ? AND sop_content_hash = ? AND prompt_version = ? AND status = ?", tenantID, sopKBID, sopDocID, contentHash, selfCheckSOPPromptVersion, "completed").First(&cached).Error; err == nil {
		var payload selfCheckSOPExtractionPayload
		if json.Unmarshal([]byte(cached.Requirements), &payload) == nil && len(payload.Requirements) > 0 {
			return payload.Requirements, cached.Summary, nil
		}
	}
	reqs, summary, err := extractSelfCheckSOPRequirements(ctx, model, sopTitle, sopText, "")
	cache := selfCheckSOPDeliverableCache{ID: uuid.NewString(), TenantID: tenantID, SOPKBID: sopKBID, SOPDocumentID: sopDocID, SOPContentHash: contentHash, PromptVersion: selfCheckSOPPromptVersion, Summary: summary, Requirements: types.JSON([]byte(`[]`))}
	if err != nil {
		cache.Status, cache.Error = "failed", err.Error()
	} else {
		payload, _ := json.Marshal(selfCheckSOPExtractionPayload{Summary: summary, Requirements: reqs})
		cache.Status, cache.Requirements = "completed", types.JSON(payload)
	}
	// A failed extraction is retained for diagnostics, but a later explicit
	// retry can replace it because only completed rows are read as cache hits.
	_ = h.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_id"},
			{Name: "sop_kb_id"},
			{Name: "sop_document_id"},
			{Name: "sop_content_hash"},
			{Name: "prompt_version"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"status", "summary", "requirements", "error", "updated_at"}),
	}).Create(&cache).Error
	return reqs, summary, err
}

func summarizeSelfCheckProject(ctx context.Context, model chat.Chat, projectName, projectText string) (string, error) {
	response, err := model.Chat(ctx, []chat.Message{
		{Role: "system", Content: "你是企业项目材料分析助手。请只根据给定材料概括项目目标、当前可识别阶段、已有交付物类型及关键词。不得补造事实。输出不超过 800 字的纯文本摘要。"},
		{Role: "user", Content: "【项目知识库】" + projectName + "\n" + projectText},
	}, &chat.ChatOptions{Temperature: 0, MaxTokens: 1200})
	if err != nil {
		return "", err
	}
	summary := strings.TrimSpace(response.Content)
	if summary == "" {
		return "", fmt.Errorf("模型未返回项目材料摘要")
	}
	return summary, nil
}
func extractSelfCheckSOPRequirements(ctx context.Context, model chat.Chat, sopName, sopText, projectSummary string) ([]selfCheckSOPRequirement, string, error) {
	if rules := extractSelfCheckSOPRequirementsByRules(sopText); len(rules) > 0 {
		return rules, "按 SOP 产出物小节列表抽取实体交付物", nil
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		feedback := ""
		if lastErr != nil {
			feedback = "\n上一轮输出解析失败：" + lastErr.Error() + "。请只返回完整 JSON。"
		}
		response, err := model.Chat(ctx, []chat.Message{
			{Role: "system", Content: "你是企业 SOP 交付物抽取器。只抽取需要在项目知识库中归档、或可被文档标题/目录证明存在的实体交付物。\n" +
				"规则：\n" +
				"1. 优先读取“输入及产出物/产出物/交付物/输出物”小节。\n" +
				"2. 不抽取“附件模板”小节及其下属模板。\n" +
				"3. 不抽取动作、流程、时限、角色职责、评审规则。\n" +
				"4. 如果一个交付包包含多个子交付物，请保留为一个 requirement，并写成“交付包（包含：子项1、子项2）”。\n" +
				"5. 只返回 JSON，不要 Markdown，不要解释文字。\n" +
				"格式：{\"summary\":\"一句话说明本 SOP 的实体交付物范围\",\"requirements\":[{\"requirement\":\"交付物或交付包名称\",\"sop_clause\":\"来自 SOP 的阶段/章节依据\",\"condition\":\"适用条件；没有则为空\"}]}"},
			{Role: "user", Content: "SOP 名称：" + sopName + "\n\nSOP 全文：\n" + sopText + feedback},
		}, &chat.ChatOptions{Temperature: 0, MaxTokens: 3000})
		if err != nil {
			lastErr = err
			continue
		}
		var extracted selfCheckSOPExtractionPayload
		if err := json.Unmarshal([]byte(extractSelfCheckJSON(response.Content)), &extracted); err != nil {
			fallback := extractSelfCheckSOPRequirementsByRules(sopText)
			if len(fallback) > 0 {
				return fallback, "按 SOP 产出物章节规则抽取实体交付物", nil
			}
			lastErr = fmt.Errorf("?? SOP ??? JSON ??: %w", err)
			continue
		}
		seen := map[string]bool{}
		valid := make([]selfCheckSOPRequirement, 0, len(extracted.Requirements))
		for _, req := range extracted.Requirements {
			req.Requirement = strings.TrimSpace(req.Requirement)
			if req.Requirement == "" || seen[normalizeSelfCheckText(req.Requirement)] || isSelfCheckExcludedRequirement(req.Requirement) || isSelfCheckActionRequirement(req.Requirement) {
				continue
			}
			seen[normalizeSelfCheckText(req.Requirement)] = true
			valid = append(valid, req)
		}
		for _, req := range extractSelfCheckSOPRequirementsByRules(sopText) {
			req.Requirement = strings.TrimSpace(req.Requirement)
			key := normalizeSelfCheckText(req.Requirement)
			if req.Requirement == "" || key == "" || seen[key] || isSelfCheckExcludedRequirement(req.Requirement) || isSelfCheckActionRequirement(req.Requirement) {
				continue
			}
			seen[key] = true
			valid = append(valid, req)
		}
		if len(valid) == 0 {
			fallback := extractSelfCheckSOPRequirementsByRules(sopText)
			if len(fallback) > 0 {
				return fallback, "按 SOP 产出物章节规则抽取实体交付物", nil
			}
			lastErr = fmt.Errorf("SOP 中未抽取到可检查的实体交付物；模型返回 %d 条，过滤后 0 条", len(extracted.Requirements))
			continue
		}
		extracted.Requirements = valid
		return extracted.Requirements, extracted.Summary, nil
	}
	return nil, "", lastErr
}
func extractSelfCheckSOPRequirementsByRules(sopText string) []selfCheckSOPRequirement {
	lines := strings.Split(sopText, "\n")
	reqs := make([]selfCheckSOPRequirement, 0)
	seen := map[string]bool{}
	inOutputList := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		title := normalizeSelfCheckSectionTitle(line)
		if title == "\u9644\u4ef6\u6a21\u677f" {
			if inOutputList {
				break
			}
			continue
		}
		if title == "\u4ea7\u51fa\u7269" || title == "\u4ea4\u4ed8\u7269" || title == "\u8f93\u51fa\u7269" {
			inOutputList = true
			continue
		}
		if !inOutputList {
			continue
		}
		if isSelfCheckSectionBoundary(line) {
			break
		}
		item := selfCheckTrimListMarker(line)
		if item == "" || strings.HasPrefix(item, "|") || isSelfCheckExcludedRequirement(item) {
			continue
		}
		key := normalizeSelfCheckText(item)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		reqs = append(reqs, selfCheckSOPRequirement{Requirement: item, SOPClause: "SOP \u4ea7\u51fa\u7269\u5c0f\u8282", Condition: ""})
	}
	return reqs
}

func normalizeSelfCheckSectionTitle(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimLeft(line, "# ")
	line = strings.TrimSpace(line)
	line = trimSelfCheckNumberedPrefix(line)
	return strings.TrimSpace(line)
}

func trimSelfCheckNumberedPrefix(line string) string {
	runes := []rune(strings.TrimSpace(line))
	i := 0
	for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(runes) {
		return string(runes)
	}
	if runes[i] != '.' && runes[i] != '）' && runes[i] != '、' {
		return string(runes)
	}
	i++
	for i < len(runes) && (runes[i] == ' ' || runes[i] == '\t') {
		i++
	}
	return string(runes[i:])
}

func isSelfCheckSectionBoundary(line string) bool {
	title := normalizeSelfCheckSectionTitle(line)
	if title == "" {
		return false
	}
	if strings.HasPrefix(line, "#") {
		return title != "\u4ea7\u51fa\u7269" && title != "\u4ea4\u4ed8\u7269" && title != "\u8f93\u51fa\u7269"
	}
	if trimSelfCheckNumberedPrefix(line) != strings.TrimSpace(line) {
		return title != "\u4ea7\u51fa\u7269" && title != "\u4ea4\u4ed8\u7269" && title != "\u8f93\u51fa\u7269"
	}
	switch title {
	case "\u9644\u4ef6\u6a21\u677f", "\u5f02\u5e38\u5904\u7406", "\u5173\u8054\u8d44\u6e90", "\u8f93\u5165", "\u89d2\u8272\u4e0e\u804c\u8d23", "\u6d41\u7a0b\u6b65\u9aa4":
		return true
	default:
		return false
	}
}

func selfCheckTrimListMarker(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimLeft(line, "-* \t")
	line = strings.TrimPrefix(line, "\u2022")
	line = strings.TrimPrefix(line, "\u00b7")
	line = strings.TrimPrefix(line, "\u3001")
	return strings.TrimSpace(line)
}

func parseSelfCheckOutputLine(line string) []string {
	item := selfCheckTrimListMarker(line)
	if item == "" || strings.HasPrefix(item, "|") {
		return nil
	}
	return []string{item}
}

func splitSelfCheckArtifacts(text string) []string {
	item := cleanSelfCheckArtifact(text)
	if item == "" || isSelfCheckExcludedRequirement(item) {
		return nil
	}
	return []string{item}
}

func cleanSelfCheckArtifact(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\uff0c,\u3002\u3001\uff1b;\uff1a:| \t")
	value = strings.TrimPrefix(value, "\u4ea7\u51fa\u7269")
	value = strings.TrimPrefix(value, "\u4ea4\u4ed8\u7269")
	return strings.TrimSpace(value)
}

func isSelfCheckActionRequirement(value string) bool {
	v := strings.TrimSpace(value)
	lower := strings.ToLower(v)
	entityWords := []string{"\u6587\u6863", "\u8868", "\u8bb0\u5f55", "\u622a\u56fe", "\u6e05\u5355", "\u62a5\u544a", "\u53f0\u8d26", "\u56fe", "\u539f\u578b", "\u770b\u677f", "story board", "storyboard", "prd", "\u65b9\u6848", "ppt", "\u6f14\u793a\u7a3f", "\u5e7b\u706f\u7247", "deck"}
	actionWords := []string{"\u5b9a\u4e49", "\u786e\u8ba4", "\u7ed8\u5236", "\u7f16\u5199", "\u8d70\u67e5", "\u8bc4\u5ba1", "\u68c0\u67e5", "\u63d0\u4ea4", "\u901a\u77e5", "\u56de\u8bbf", "\u5904\u7406", "\u7ef4\u62a4", "\u66f4\u65b0", "\u5b8c\u6210", "\u8fdb\u5165", "\u4e0d\u5f97", "\u9700\u8981", "\u5fc5\u987b", "\u5e94\u5f53"}
	return containsAny(lower, actionWords) && !containsAny(lower, entityWords)
}

func isSelfCheckExcludedRequirement(value string) bool {
	return strings.Contains(strings.TrimSpace(value), "\u9644\u4ef6\u6a21\u677f")
}

// Models occasionally wrap valid JSON in prose or markdown fences. Keep the
// contract strict while tolerating those presentation wrappers.
func extractSelfCheckJSON(content string) string {
	raw := strings.TrimSpace(content)
	fence := string([]byte{96, 96, 96})
	raw = strings.TrimPrefix(raw, fence+"json")
	raw = strings.TrimPrefix(raw, fence)
	raw = strings.TrimSuffix(raw, fence)
	if start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}"); start >= 0 && end > start {
		return strings.TrimSpace(raw[start : end+1])
	}
	return raw
}

func generateSelfCheckAnalysis(ctx context.Context, model chat.Chat, profile selfCheckProfile, projectKB, sopKB types.KnowledgeBase, projectText, projectSummary string, requirements []selfCheckSOPRequirement, sopSummary string) (selfCheckAIResult, error) {
	const batchSize = 5
	merged := selfCheckAIResult{}
	for start, batch := 0, 1; start < len(requirements); start, batch = start+batchSize, batch+1 {
		end := start + batchSize
		if end > len(requirements) {
			end = len(requirements)
		}
		result, err := generateSelfCheckAnalysisBatch(ctx, model, profile, projectKB, sopKB, projectText, projectSummary, requirements[start:end], sopSummary, batch)
		if strings.TrimSpace(result.ProjectSummary) != "" {
			merged.ProjectSummary = result.ProjectSummary
		}
		if strings.TrimSpace(result.SOPSummary) != "" {
			merged.SOPSummary = result.SOPSummary
		}
		merged.Findings = append(merged.Findings, result.Findings...)
		if err != nil {
			logger.Errorf(ctx, "[KnowledgeSelfCheck] analysis batch failed but fallback findings were generated batch=%d batch_requirements=%d error=%v", batch, end-start, err)
		}
	}
	if len(merged.Findings) == 0 {
		return merged, fmt.Errorf("自检模型未返回任何可保存的检查结果")
	}
	return merged, nil
}

func generateSelfCheckAnalysisBatch(ctx context.Context, model chat.Chat, profile selfCheckProfile, projectKB, sopKB types.KnowledgeBase, projectText, projectSummary string, requirements []selfCheckSOPRequirement, sopSummary string, batch int) (selfCheckAIResult, error) {
	contextData, _ := json.Marshal(map[string]string{"project_name": projectKB.Name, "owner": profile.OwnerName, "sop_name": sopKB.Name})
	requirementsJSON, _ := json.Marshal(requirements)
	prompt := "项目属性：" + string(contextData) +
		"\n\n【项目文档摘要】\n" + projectSummary +
		"\n\n【项目文档全文】\n" + projectText +
		"\n\n【本批次必须逐项检查的 SOP requirements】\n" + string(requirementsJSON) +
		"\n\n【SOP 摘要】\n" + sopSummary +
		"\n\n请只检查本批次 requirements，findings 数量必须等于本批次 requirements 数量。"
	response, err := model.Chat(ctx, []chat.Message{
		{Role: "system", Content: "你是企业项目交付物审计员。任务是根据给定 SOP requirements 逐项核对项目文档是否已经交付。\n" +
			"只返回 JSON，不要 Markdown，不要解释文字。JSON 必须完整闭合，可被 json.Unmarshal 直接解析。\n" +
			"findings 数量必须等于 requirements 数量，不得新增、合并或遗漏。\n" +
			"state 只能是 present、partial、missing、uncertain。\n" +
			"present：项目文档明确包含该交付物。\n" +
			"partial：找到相关内容，但只命中部分子项或证据不足以覆盖完整交付包。\n" +
			"missing：项目文档没有找到该交付物。\n" +
			"uncertain：项目文档证据不足，不能可靠判断。\n" +
			"返回格式：{\"project_summary\":\"项目文档摘要\",\"sop_summary\":\"本批次适用 SOP 摘要\",\"findings\":[{\"requirement\":\"交付物要求\",\"material\":\"对应项目材料或空字符串\",\"state\":\"present|partial|missing|uncertain\",\"evidence\":\"项目文档中的判断依据；没有证据则写未找到明确证据\",\"sop_clause\":\"SOP 条款依据\",\"suggestion\":\"需要补充的内容；present 则留空\"}]}"},
		{Role: "user", Content: prompt},
	}, &chat.ChatOptions{Temperature: 0, MaxTokens: 2200})
	if err != nil {
		return fallbackSelfCheckAIResult(projectSummary, sopSummary, requirements, fmt.Sprintf("模型调用失败：%v", err)), err
	}
	raw := extractSelfCheckJSON(response.Content)
	var result selfCheckAIResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		logger.Errorf(ctx, "[KnowledgeSelfCheck] analysis JSON parse failed batch=%d response_length=%d has_json_fence=%t prefix=%q suffix=%q error=%v", batch, len(response.Content), strings.Contains(response.Content, string([]byte{96, 96, 96})+"json"), responsePrefix(response.Content), responseSuffix(response.Content), err)
		return fallbackSelfCheckAIResult(projectSummary, sopSummary, requirements, fmt.Sprintf("模型返回 JSON 解析失败：%v；response_length=%d；response_preview=%s", err, len(response.Content), responsePrefix(response.Content))), err
	}
	if len(result.Findings) < len(requirements) {
		logger.Errorf(ctx, "[KnowledgeSelfCheck] analysis returned fewer findings batch=%d expected=%d actual=%d", batch, len(requirements), len(result.Findings))
		for i := len(result.Findings); i < len(requirements); i++ {
			result.Findings = append(result.Findings, selfCheckAIFinding{
				Requirement: requirements[i].Requirement,
				State:       "uncertain",
				Evidence:    "检查模型未返回该交付物结果",
				SOPClause:   requirements[i].SOPClause,
				Suggestion:  "请重试或人工确认该交付物",
			})
		}
	}
	for i := range result.Findings {
		result.Findings[i].State = normalizeSelfCheckState(result.Findings[i].State)
		if strings.TrimSpace(result.Findings[i].Requirement) == "" && i < len(requirements) {
			result.Findings[i].Requirement = requirements[i].Requirement
		}
		if strings.TrimSpace(result.Findings[i].SOPClause) == "" && i < len(requirements) {
			result.Findings[i].SOPClause = requirements[i].SOPClause
		}
	}
	sort.SliceStable(result.Findings, func(i, j int) bool { return result.Findings[i].State < result.Findings[j].State })
	return result, nil
}
func normalizeSelfCheckState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "present", "complete", "completed", "pass", "passed":
		return "present"
	case "partial", "incomplete":
		return "partial"
	case "missing", "absent":
		return "missing"
	default:
		return "uncertain"
	}
}

func fallbackSelfCheckAIResult(projectSummary, sopSummary string, requirements []selfCheckSOPRequirement, reason string) selfCheckAIResult {
	result := selfCheckAIResult{ProjectSummary: projectSummary, SOPSummary: sopSummary}
	for _, requirement := range requirements {
		result.Findings = append(result.Findings, selfCheckAIFinding{
			Requirement: requirement.Requirement,
			State:       "uncertain",
			Evidence:    reason,
			SOPClause:   requirement.SOPClause,
			Suggestion:  "模型响应异常，已保留该 requirement 的待确认检查项；请重试或人工复核。",
		})
	}
	return result
}
func responsePrefix(value string) string {
	if len(value) <= 500 {
		return value
	}
	return value[:500]
}
func responseSuffix(value string) string {
	if len(value) <= 500 {
		return value
	}
	return value[len(value)-500:]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "未命名文档"
}

// StartKnowledgeSelfCheckScheduler runs enabled project self-checks once per day
// at each profile's configured local time. Each run persists findings and sends
// the daily result through the configured IM channel.
func (h *MessageHandler) StartKnowledgeSelfCheckScheduler(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				h.runDueKnowledgeSelfChecks(ctx, now)
			}
		}
	}()
}

func normalizeSelfCheckDailySchedule(schedule string) (string, error) {
	schedule = strings.ToLower(strings.TrimSpace(schedule))
	if schedule == "" {
		return "daily_0900", nil
	}
	parts := strings.Split(schedule, "_")
	if len(parts) != 2 || parts[0] != "daily" || len(parts[1]) != 4 {
		return "", fmt.Errorf("unsupported self-check schedule: %s", schedule)
	}
	hour, hourErr := strconv.Atoi(parts[1][:2])
	minute, minuteErr := strconv.Atoi(parts[1][2:])
	if hourErr != nil || minuteErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return "", fmt.Errorf("invalid self-check schedule time: %s", schedule)
	}
	return fmt.Sprintf("daily_%02d%02d", hour, minute), nil
}

func selfCheckDailyScheduleSlot(schedule string, localNow time.Time) (time.Time, bool) {
	normalized, err := normalizeSelfCheckDailySchedule(schedule)
	if err != nil {
		return time.Time{}, false
	}
	hour, _ := strconv.Atoi(normalized[6:8])
	minute, _ := strconv.Atoi(normalized[8:])
	if localNow.Hour() != hour || localNow.Minute() < minute {
		return time.Time{}, false
	}
	return time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hour, minute, 0, 0, localNow.Location()).UTC(), true
}

func (h *MessageHandler) runDueKnowledgeSelfChecks(ctx context.Context, now time.Time) {
	var profiles []selfCheckProfile
	if err := h.db.WithContext(ctx).Where("purpose = ? AND enabled = ?", "project", true).Find(&profiles).Error; err != nil {
		return
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	localNow := now.In(loc)
	for _, profile := range profiles {
		startOfSlot, due := selfCheckDailyScheduleSlot(profile.DailySchedule, localNow)
		if !due {
			continue
		}
		if profile.LastRunAt != nil && !profile.LastRunAt.Before(startOfSlot) {
			continue
		}
		tenantCtx := context.WithValue(ctx, types.TenantIDContextKey, profile.TenantID)
		if _, _, err := h.runKnowledgeSelfCheck(tenantCtx, profile.TenantID, profile.KnowledgeBaseID, ""); err != nil {
			logger.Warnf(ctx, "[KnowledgeSelfCheck] daily run failed tenant=%d kb=%s error=%v", profile.TenantID, profile.KnowledgeBaseID, err)
			continue
		}
		nowUTC := time.Now().UTC()
		_ = h.db.WithContext(ctx).Model(&selfCheckProfile{}).
			Where("tenant_id = ? AND knowledge_base_id = ?", profile.TenantID, profile.KnowledgeBaseID).
			Update("last_run_at", nowUTC).Error
	}
}
