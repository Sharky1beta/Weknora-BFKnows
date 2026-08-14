package handler

import (
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestIsMonitoredQualityAnswer(t *testing.T) {
	tests := []struct {
		name   string
		answer qualityMessageRow
		want   bool
	}{
		{name: "ordinary chat excluded", answer: qualityMessageRow{}, want: false},
		{name: "fallback included", answer: qualityMessageRow{IsFallback: true}, want: true},
		{name: "reference included", answer: qualityMessageRow{KnowledgeReferences: types.References{&types.SearchResult{ID: "ref-1"}}}, want: true},
		{name: "knowledge scope included", answer: qualityMessageRow{ExecutionContext: types.MessageExecutionContext{KnowledgeBaseIDs: []string{"kb-1"}}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMonitoredQualityAnswer(tt.answer); got != tt.want {
				t.Fatalf("isMonitoredQualityAnswer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClassifyQualityAnswer(t *testing.T) {
	reference := types.References{&types.SearchResult{ID: "ref-1"}}
	tests := []struct {
		name       string
		answer     qualityMessageRow
		wantHit    bool
		wantReason string
	}{
		{
			name:       "fallback is a miss",
			answer:     qualityMessageRow{IsFallback: true, KnowledgeReferences: reference},
			wantReason: "fallback",
		},
		{
			name:       "no reference is a miss",
			answer:     qualityMessageRow{},
			wantReason: "no_knowledge_reference",
		},
		{
			name: "referenced abstention is a miss",
			answer: qualityMessageRow{
				KnowledgeReferences: reference,
				Answer:              "根据提供的检索信息，并未包含火星基地采购项目 XJ-2099 的任何内容，因此无法告知负责人。",
			},
			wantReason: "explicit_no_answer",
		},
		{
			name: "referenced answer is a hit",
			answer: qualityMessageRow{
				KnowledgeReferences: reference,
				Answer:              "TCP 是面向连接的传输层协议，提供可靠、有序的数据传输。",
			},
			wantHit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hit, reason := classifyQualityAnswer(tt.answer)
			if hit != tt.wantHit || reason != tt.wantReason {
				t.Fatalf("classifyQualityAnswer() = (%v, %q), want (%v, %q)", hit, reason, tt.wantHit, tt.wantReason)
			}
		})
	}
}

func TestIsExplicitKnowledgeMissAvoidsBroadNegationFalsePositive(t *testing.T) {
	answer := "UDP 没有连接建立过程，因此首包延迟通常更低。"
	if isExplicitKnowledgeMiss(answer) {
		t.Fatalf("ordinary factual negation was classified as a miss: %q", answer)
	}
}

func TestSummarizeQualityTurns(t *testing.T) {
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	turns := []qualityTurn{
		{Hit: true, Channel: "web", ResponseTimeMs: 100, FirstResponseTimeMs: 40},
		{Hit: true, Channel: "im", ResponseTimeMs: 200, FirstResponseTimeMs: 60},
		{Hit: false, Channel: "IM", ResponseTimeMs: 1000, FirstResponseTimeMs: 100},
		{Hit: false, Channel: "", ResponseTimeMs: 0},
	}

	got := summarizeQualityTurns(turns, from, to)
	if got.Total != 4 || got.Hits != 2 || got.Misses != 2 {
		t.Fatalf("unexpected counts: total=%d hits=%d misses=%d", got.Total, got.Hits, got.Misses)
	}
	if got.HitRate != 50 {
		t.Fatalf("hit rate = %v, want 50", got.HitRate)
	}
	if got.AverageResponseTimeMs != 433 || got.P95ResponseTimeMs != 1000 {
		t.Fatalf("unexpected response latency: avg=%d p95=%d", got.AverageResponseTimeMs, got.P95ResponseTimeMs)
	}
	if got.ByChannel["im"].Total != 2 || got.ByChannel["im"].HitRate != 50 {
		t.Fatalf("unexpected IM summary: %#v", got.ByChannel["im"])
	}
	if got.ByChannel["unknown"].Misses != 1 {
		t.Fatalf("unknown channel miss was not counted")
	}
}

func TestParseQualityWindow(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	from, to, err := parseQualityWindow("", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if !to.Equal(now) || to.Sub(from) != qualityDefaultWindow {
		t.Fatalf("unexpected default window: %s - %s", from, to)
	}

	if _, _, err := parseQualityWindow("2026-01-01T00:00:00Z", "2026-07-30T00:00:00Z", now); err == nil {
		t.Fatal("expected ranges over 90 days to fail")
	}
	if _, _, err := parseQualityWindow("bad", "", now); err == nil {
		t.Fatal("expected malformed from timestamp to fail")
	}
}

func TestParseQualityPagination(t *testing.T) {
	page, size, err := parseQualityPagination("", "")
	if err != nil || page != 1 || size != 50 {
		t.Fatalf("unexpected defaults: page=%d size=%d err=%v", page, size, err)
	}
	if _, _, err := parseQualityPagination("0", "50"); err == nil {
		t.Fatal("expected page=0 to fail")
	}
	if _, _, err := parseQualityPagination("1", "201"); err == nil {
		t.Fatal("expected page_size=201 to fail")
	}
}
