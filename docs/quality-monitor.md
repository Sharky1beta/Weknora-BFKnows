# Knowledge Q&A quality monitoring

WeKnora calculates knowledge-answer quality directly from persisted chat messages. Reading these metrics does not call an LLM and does not consume model tokens.

## Metric definitions

- **Monitored turn:** the assistant turn selected a knowledge scope, returned at least one knowledge reference, or triggered the knowledge fallback.
- **Hit:** the completed assistant turn contains at least one knowledge reference, did not trigger fallback, and did not explicitly state that the retrieved knowledge cannot answer the question.
- **Miss:** a monitored turn triggered fallback, completed without a knowledge reference, or explicitly stated that the retrieved knowledge does not contain an answer. This last case prevents irrelevant references from becoming false-positive hits.
- **Response time:** elapsed milliseconds from creation of the paired user message to the final update of the completed assistant message.
- **First-response time:** the existing `agent_duration_ms` value when the channel records it. Missing or zero values are excluded from the first-response aggregate.

Ordinary conversation without a knowledge scope is excluded from the denominator.

## API

Both endpoints require a tenant administrator (or an equivalent full-access API key).

### Summary

```http
GET /api/v1/quality-monitor/summary
```

Optional query parameters:

- `from`, `to`: RFC3339 timestamps. Defaults to the latest seven days; maximum range is 90 days.
- `agent_id`: include only one agent.
- `channel`: include only one source channel, for example `web`, `api`, or `im`.

The response contains totals, hit rate, average and P95 latency, and a per-channel breakdown.

### Missed questions

```http
GET /api/v1/quality-monitor/misses?page=1&page_size=50
```

This endpoint accepts the same time, agent, and channel filters. It returns the original question, answer, session, channel, latency, reference count, and miss reason. `page_size` may not exceed 200.

These records can later be exported to a Feishu Bitable without re-running question answering.

## AI quality report

The **Quality monitoring** page also generates an executive-style report from the stored metrics. Opening the page and reading metrics do not call a model. A model is called only when a report is generated manually or by the weekly schedule.

Each report contains:

- the current and previous-period hit rate, miss count, and response latency;
- the ten most frequent normalized questions;
- knowledge gaps grouped by miss reason and source channel;
- an AI-written executive summary, key findings, actionable recommendations, follow-up questions, and data caveats.

The default window is the latest seven days. Automatic reports run every Monday at 09:00 in `Asia/Shanghai`. Automatic sending remains disabled until both an enabled Feishu IM channel and a project-group `chat_id` are configured.

### Report API

All endpoints require a tenant administrator (or an equivalent full-access API key).

```http
GET  /api/v1/quality-monitor/report-config
PUT  /api/v1/quality-monitor/report-config
POST /api/v1/quality-monitor/reports/generate
GET  /api/v1/quality-monitor/reports
POST /api/v1/quality-monitor/reports/:id/send
```

The generated report is persisted before any outbound send attempt. Send status and errors remain available in report history for auditing and retry.
