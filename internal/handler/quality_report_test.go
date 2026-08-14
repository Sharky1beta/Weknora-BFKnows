package handler

import (
	"strings"
	"testing"
	"time"
)

func TestAggregateQualityReport(t *testing.T) {
	turns := []qualityTurn{
		{Question: "如何申请采购？", Hit: true, Channel: "web"},
		{Question: "  如何申请采购？ ", Hit: false, Reason: "explicit_no_answer", Channel: "web"},
		{Question: "项目验收负责人是谁？", Hit: false, Reason: "no_knowledge_reference", Channel: "im"},
	}

	frequent, reasons, channels := aggregateQualityReport(turns)
	if len(frequent) != 2 || frequent[0].Question != "如何申请采购？" {
		t.Fatalf("unexpected frequent questions: %#v", frequent)
	}
	if frequent[0].Count != 2 || frequent[0].Hits != 1 || frequent[0].Misses != 1 {
		t.Fatalf("unexpected top question counts: %#v", frequent[0])
	}
	if len(reasons) != 2 || reasons[0].Count != 1 || reasons[1].Count != 1 {
		t.Fatalf("unexpected reason gaps: %#v", reasons)
	}
	if len(channels) != 2 || channels[0].Count != 1 || channels[1].Count != 1 {
		t.Fatalf("unexpected channel gaps: %#v", channels)
	}
}

func TestFormatQualityReportMessage(t *testing.T) {
	from := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	report := qualityReportPayload{
		Title: "知识问答质量周报",
		Current: qualitySummary{
			From: from, To: from.Add(7 * 24 * time.Hour), Total: 10,
			Hits: 7, Misses: 3, HitRate: 70, AverageResponseTimeMs: 1250,
		},
		HitRateChange: 5,
		Analysis: qualityAIAnalysis{
			ExecutiveSummary: "命中率较上期改善。",
			KeyFindings:      []string{"未命中集中在项目交付类问题。"},
			Recommendations:  []string{"补充项目验收负责人字段。"},
		},
	}

	message := formatQualityReportMessage(report)
	for _, expected := range []string{"知识问答质量周报", "命中率：70.0%", "关键发现", "改进建议", "补充项目验收负责人字段"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("formatted report is missing %q: %s", expected, message)
		}
	}
}

func TestNormalizeQualityReportSchedule(t *testing.T) {
	got, err := normalizeQualityReportSchedule("weekly_wednesday_1430")
	if err != nil || got != "weekly_wednesday_1430" {
		t.Fatalf("normalizeQualityReportSchedule() = %q, %v", got, err)
	}
	for _, value := range []string{"daily_0900", "weekly_funday_0900", "weekly_monday_2460"} {
		if _, err := normalizeQualityReportSchedule(value); err == nil {
			t.Fatalf("expected %q to fail", value)
		}
	}
}

func TestQualityReportScheduleSlot(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	wednesday := time.Date(2026, 8, 5, 14, 45, 0, 0, loc)
	slot, due := qualityReportScheduleSlot("weekly_wednesday_1430", wednesday)
	if !due {
		t.Fatal("weekly schedule should be due on Wednesday after 14:30")
	}
	want := time.Date(2026, 8, 5, 14, 30, 0, 0, loc).UTC()
	if !slot.Equal(want) {
		t.Fatalf("slot = %v, want %v", slot, want)
	}
	if _, due := qualityReportScheduleSlot("weekly_wednesday_1430", wednesday.Add(24*time.Hour)); due {
		t.Fatal("weekly schedule should not be due on Thursday")
	}
	if _, due := qualityReportScheduleSlot("weekly_wednesday_1430", time.Date(2026, 8, 5, 14, 29, 0, 0, loc)); due {
		t.Fatal("weekly schedule should not be due before 14:30")
	}
}
