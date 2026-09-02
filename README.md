# BEDA AI Inquiry Processing System

## Overview

BEDA receives unstructured business inquiries from email, web forms, and messaging channels. These may represent sales opportunities, support requests, duplicate messages, spam, or incomplete submissions.

The goal is to classify and structure each inquiry, resolve customer identity where possible, determine a safe action, and maintain a clear human approval boundary for consequential decisions.

This prototype focuses on the assessment flow:

intake → classification → extraction → validation → CRM resolution → policy → approval → execution → audit trail

---

## Architecture

The implementation follows a thin-boundary design:

```text
HTTP/API
  → Workflow
  → AI Provider
  → Validation
  → CRM
  → Policy
  → Approval
  → Executor
  → Audit
```

The AI provider is intentionally mocked for the assessment. The workflow layer remains deterministic where decisions matter, and the application layer owns policy and execution authority.

The AI-assisted parts are classification and extraction. The deterministic parts include validation, duplicate detection, CRM matching, policy enforcement, approval state transitions, idempotent execution, and audit logging.

---

## Safety Principle

> The model can recommend. The application decides.

This is the core rule of the prototype:

- The LLM is untrusted input and must not be treated as truth.
- Deterministic validation checks schema and confidence constraints before a result is accepted.
- Policy decides whether an action is ALLOW, REQUIRE_APPROVAL, or DENY.
- Human approval is required for consequential actions when policy requires it.
- Idempotency prevents duplicate processing and repeated execution.
- Audit events preserve the workflow trail for accountability.

---

## Run locally

```bash
go test ./...
go run ./cmd/api
```

The API listens on port 8080.

---

## API examples

### Submit an inquiry

```bash
curl -X POST http://localhost:8080/api/v1/inquiries \
  -H "Content-Type: application/json" \
  -d '{
    "source": "email",
    "external_message_id": "msg-123",
    "sender": {
      "email": "customer@example.com",
      "name": "Customer"
    },
    "subject": "Interested in sales automation",
    "content": "We are a team of 20 and want to improve our sales process. Please contact us."
  }'
```

### Approve an action

Use the action ID returned in the inquiry response from the previous request.

```bash
curl -X POST http://localhost:8080/api/v1/actions/{ACTION_ID}/approve \
  -H "Content-Type: application/json" \
  -d '{
    "approver_id": "human-reviewer"
  }'
```

Approval changes the action state to `APPROVED`. Execution is a separate, controlled transition that happens only after approval is granted.

### Reject an action

```bash
curl -X POST http://localhost:8080/api/v1/actions/{ACTION_ID}/reject \
  -H "Content-Type: application/json" \
  -d '{
    "approver_id": "human-reviewer",
    "reason": "not approved"
  }'
```

---

## Prototype Limitations

This is a prototype, not production infrastructure.

- AI provider is mocked.
- CRM is mocked.
- Repositories are in-memory.
- No production authentication or RBAC yet.
- No durable database yet.
- External integrations are not connected.
- This is a demonstrable vertical slice, not a production deployment.
