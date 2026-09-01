# BEDA AI Internship Technical Assessment Prototype

This repository contains a minimal Go prototype for the BEDA AI intake workflow described in the project documents. The system handles unstructured inquiry intake, classification, extraction, deterministic validation, CRM identity matching, policy evaluation, and audit logging.

## What the prototype does

The prototype is intentionally narrow and deliberately avoids real external integrations.

It demonstrates:

- external inquiry ingestion via HTTP
- request validation
- workflow execution through a Go service layer
- mock LLM classification and extraction
- deterministic validation of model output
- mock CRM matching
- policy decisions for ALLOW, REQUIRE_APPROVAL, and DENY
- idempotency for duplicate inquiries by `(source, external_message_id)`
- audit record generation for important transitions

## Current limitations

This prototype uses:

- mock providers for LLM and CRM behavior
- in-memory state for deduplication and audit records
- no PostgreSQL persistence
- no real external API integrations
- no authentication or authorization layer
- no real background worker or queue
- no real approval workflow beyond the prototype decision boundary

## How to run

```bash
go run ./cmd/api
```

The server listens on port 8080.

## How to run tests

```bash
go test ./...
```

## Example curl request

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
    "subject": "We need support automation",
    "content": "We want to automate our customer support process."
  }'
```

## Security and design notes

- LLM output is treated as untrusted input.
- The HTTP layer validate only structural request correctness.
- Policy decisions are kept deterministic and independent from the model.
- The LLM is never allowed to directly execute business actions.
- The current prototype is designed to be demonstrable and testable without production infrastructure.
