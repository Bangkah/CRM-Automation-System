# System Architecture — BEDA AI Intake System

## 1. Prinsip arsitektur

Kita jadikan ini sebagai prinsip utama:

> **LLM menghasilkan intelligence; application layer memegang authority.**

Artinya LLM boleh:

* memahami pesan,
* mengklasifikasikan,
* mengekstrak data,
* melakukan reasoning,
* membuat draft,
* memberikan rekomendasi.

Tetapi LLM **tidak boleh langsung**:

* mengubah CRM,
* mengirim email,
* menghapus data,
* mengubah permission,
* menjalankan arbitrary API,
* menjalankan shell command.

Semua tindakan melewati **Policy & Action Layer**.

---

# 2. Arsitektur tingkat tinggi

```text
                         EXTERNAL CHANNELS
                  ┌─────────┬─────────┬─────────┐
                  │  Email  │  Forms  │ Message │
                  └────┬────┴────┬────┴────┬────┘
                       │         │         │
                       └─────────┼─────────┘
                                 ▼
                    ┌────────────────────────┐
                    │      INGESTION API     │
                    │ auth / rate limit      │
                    │ idempotency            │
                    └────────────┬───────────┘
                                 │
                                 ▼
                    ┌────────────────────────┐
                    │   NORMALIZATION        │
                    │ canonical inquiry      │
                    └────────────┬───────────┘
                                 │
                                 ▼
                    ┌────────────────────────┐
                    │     AI ANALYSIS         │
                    │ classification          │
                    │ extraction              │
                    │ summarization           │
                    └────────────┬───────────┘
                                 │
                                 ▼
                    ┌────────────────────────┐
                    │ VALIDATION + CONFIDENCE │
                    │ schema / policy checks  │
                    └────────────┬───────────┘
                                 │
                  ┌──────────────┼──────────────┐
                  │              │              │
                  ▼              ▼              ▼
              COMPLETE       MISSING         LOW CONF.
                  │              │              │
                  │              ▼              ▼
                  │        Research /       Human Review
                  │        clarification
                  │              │
                  └──────────────┼──────────────┘
                                 ▼
                    ┌────────────────────────┐
                    │    CRM RESOLUTION      │
                    │ find / match / propose │
                    └────────────┬───────────┘
                                 │
                                 ▼
                    ┌────────────────────────┐
                    │    POLICY ENGINE        │
                    │ authorization           │
                    │ risk classification     │
                    └────────────┬───────────┘
                                 │
                    ┌────────────┴────────────┐
                    ▼                         ▼
              LOW RISK                  CONSEQUENTIAL
                    │                         │
                    ▼                         ▼
              AUTO ACTION              HUMAN APPROVAL
                    │                         │
                    └────────────┬────────────┘
                                 ▼
                    ┌────────────────────────┐
                    │ ACTION EXECUTOR        │
                    │ CRM / notification     │
                    │ messaging              │
                    └────────────┬───────────┘
                                 │
                                 ▼
                    ┌────────────────────────┐
                    │       AUDIT LOG         │
                    │ immutable event trail   │
                    └────────────────────────┘
```

---

# 3. Komponen utama

## A. Ingestion API

Tanggung jawab:

* menerima event dari channel;
* autentikasi webhook;
* rate limiting;
* validasi basic payload;
* membuat `inquiry_id`;
* idempotency.

Contoh:

```json
{
  "source": "email",
  "external_message_id": "gmail_93821",
  "sender": {
    "email": "john@example.com"
  },
  "content": "I'm interested in..."
}
```

### Kenapa idempotency penting?

Webhook bisa terkirim dua kali.

Tanpa idempotency:

```text
email
 ↓
CRM CREATE
 ↓
email retry
 ↓
CRM CREATE
```

Hasil:

**duplicate customer.**

Dengan:

```text
source + external_message_id
```

kita bisa mendeteksi bahwa event sudah pernah diproses.

---

# 4. Normalization Layer

Semua channel diubah menjadi satu format:

```json
{
  "inquiry_id": "inq_01",
  "source": "email",
  "received_at": "...",
  "sender": {
    "name": "John",
    "email": "john@example.com"
  },
  "subject": "...",
  "content": "...",
  "attachments": []
}
```

Jadi AI layer tidak perlu tahu apakah input berasal dari Gmail atau website.

Ini juga membuat sistem mudah diperluas:

```text
WhatsApp
Slack
Telegram
CRM webhook
        ↓
   Adapter
        ↓
Canonical Inquiry
```

---

# 5. AI Analysis Service

Saya **tidak akan membuat satu autonomous agent yang melakukan semuanya**.

Lebih baik pipeline terkontrol:

```text
Inquiry
   ↓
Classification
   ↓
Extraction
   ↓
Reasoning / missing information
   ↓
Recommendation
```

LLM menghasilkan structured output.

Contoh:

```json
{
  "category": "sales",
  "confidence": 0.94,
  "intent": "process_automation",
  "entities": {
    "company": "Example Corp",
    "team_size": 20,
    "timeline": null
  },
  "missing_information": [
    "timeline"
  ]
}
```

Kemudian application layer melakukan validation.

---

# 6. Kenapa structured output?

Kita **tidak ingin** downstream menerima:

```text
"I think this is probably a sales lead..."
```

Kita ingin:

```json
{
  "category": "sales",
  "confidence": 0.94
}
```

Kemudian schema validator memastikan:

```text
category ∈ allowed categories
confidence ∈ [0,1]
```

Kalau tidak valid:

```text
LLM output
   ↓
Schema validation ❌
   ↓
Retry / human review
```

---

# 7. Research Service

Research **tidak dijalankan untuk semua inquiry**.

Misalnya:

```text
company = "Acme"
website = null
```

dan kita membutuhkan informasi perusahaan.

Baru:

```text
Research Service
       ↓
Public sources
       ↓
Evidence
       ↓
LLM synthesis
```

Output harus menyimpan sumber:

```json
{
  "fact": "Company provides logistics software",
  "source": "company website",
  "retrieved_at": "...",
  "confidence": 0.91
}
```

Dengan begitu AI tidak sekadar berkata:

> “Menurut saya perusahaan ini bergerak di logistics.”

Kita tahu **asal informasinya**.

---

# 8. CRM Resolution

Ini bagian yang **jangan diberikan ke LLM sebagai authority**.

LLM boleh berkata:

> “Kemungkinan customer ini adalah John dari Acme.”

Tetapi CRM resolver yang menentukan.

Contoh:

```text
Email exact match
       ↓
Found customer #123
       ↓
MATCH
```

atau:

```text
Email tidak ditemukan
       ↓
Company domain match
       ↓
2 candidates
       ↓
AMBIGUOUS
       ↓
Human review
```

**Jangan biarkan AI memilih secara bebas ketika ada dua kandidat.**

---

# 9. Policy Engine

Ini menurut saya salah satu komponen terpenting dalam jawaban kita.

AI:

```text
"Send this response"
```

tidak cukup.

Policy Engine mengecek:

```text
WHO?
WHAT?
TO WHICH RESOURCE?
WITH WHICH PERMISSION?
WHAT RISK?
DOES IT REQUIRE APPROVAL?
```

Contoh:

```json
{
  "action": "send_external_message",
  "risk": "high",
  "requires_approval": true
}
```

Maka:

```text
BLOCK
 ↓
Human Approval
```

---

# 10. Action Executor

**LLM tidak memanggil CRM secara langsung.**

Sebaliknya:

```text
LLM
 ↓
Action Proposal
 ↓
Policy Engine
 ↓
Approved Action
 ↓
Action Executor
 ↓
CRM API
```

Action Executor hanya menerima action yang sudah lolos policy.

Misalnya:

```json
{
  "action": "update_contact",
  "contact_id": "crm_123",
  "fields": {
    "last_inquiry_at": "..."
  }
}
```

Bukan:

```text
"Hey AI, do whatever you think is necessary."
```

😂

---

# 11. Human Approval Service

Untuk tindakan tertentu:

```text
AI recommendation
       ↓
Approval Queue
       ↓
Human
   ┌───┴───┐
   ↓       ↓
Approve  Reject
   │
   ▼
Action
```

Approval harus tercatat:

```json
{
  "approver": "user_42",
  "decision": "approved",
  "timestamp": "...",
  "action_id": "act_123"
}
```

---

# 12. Audit Service

Saya ingin audit trail menjadi **first-class component**, bukan sekadar `console.log`.

Contoh event:

```json
{
  "event_id": "evt_123",
  "inquiry_id": "inq_001",
  "timestamp": "2026-09-01T10:00:00Z",
  "actor": "ai_classifier",
  "action": "classify",
  "result": "sales",
  "model": "model-x",
  "policy": "allowed"
}
```

Event berikutnya:

```json
{
  "actor": "human:user_42",
  "action": "approve",
  "target": "send_response",
  "result": "approved"
}
```

Dengan ini kita bisa menjawab:

> **Apa yang terjadi pada inquiry #123?**

---

# 13. Storage

Untuk desain awal saya pilih:

### PostgreSQL

Menyimpan:

```text
inquiries
contacts
companies
processing_runs
actions
approvals
audit_events
research_evidence
```

Kenapa PostgreSQL?

Karena datanya relational dan kita butuh:

* transactions,
* constraints,
* unique indexes,
* consistency,
* JSONB untuk hasil AI yang fleksibel.

Misalnya:

```sql
UNIQUE(source, external_message_id)
```

Ini membantu enforcement idempotency **di level database**, bukan cuma application code.

---

# 14. Queue

Untuk production architecture kita bisa menggunakan queue:

```text
Ingestion
   ↓
Queue
   ↓
Workers
```

Kenapa?

Karena AI call dan external API call bisa lambat.

Misalnya:

```text
Gmail webhook
   ↓
return 202 immediately
   ↓
queue
   ↓
AI worker
```

Jadi webhook tidak menunggu LLM selesai.

Untuk assessment, kita tidak perlu memaksakan teknologi tertentu. Kita bisa menyebut:

> **PostgreSQL-backed job queue untuk MVP; dedicated queue seperti Redis/SQS dapat digunakan ketika throughput membutuhkan separation yang lebih kuat.**

Ini lebih matang daripada langsung menambahkan Kafka hanya karena terlihat keren.

---

# 15. Model Strategy

Saya tidak akan mengunci arsitektur ke satu vendor.

Buat abstraction:

```text
LLMProvider
   │
   ├── OpenAI
   ├── Gemini
   ├── Anthropic
   └── Local model
```

Kemudian task routing:

```text
Simple classification
        ↓
small / cheap model

Complex reasoning
        ↓
stronger model
```

Ini membantu:

* cost control;
* vendor flexibility;
* fallback;
* testing.

---

# 16. Redis — Perlu atau Tidak?

**Opsional.**

Jangan masukkan Redis hanya supaya architecture kelihatan kompleks.

Bisa digunakan untuk:

* rate limiting;
* short-lived cache;
* job queue;
* idempotency cache.

Tetapi **source of truth tetap PostgreSQL**.

---

# 17. Security Boundary

Ini diagram yang bagus untuk submission:

```text
                  ┌──────────────────────┐
                  │      LLM             │
                  │ Untrusted reasoning  │
                  └──────────┬───────────┘
                             │
                       structured
                          output
                             │
                             ▼
                  ┌──────────────────────┐
                  │ VALIDATION           │
                  │ + POLICY ENGINE      │
                  └──────────┬───────────┘
                             │
                       approved action
                             │
                             ▼
                  ┌──────────────────────┐
                  │ ACTION EXECUTOR      │
                  │ least privilege      │
                  └──────────┬───────────┘
                             │
                             ▼
                         CRM/API
```

**LLM diperlakukan sebagai untrusted component.**

Ini akan menjadi poin security yang kuat.

---

# 18. ADR-001 — Kenapa bukan Autonomous Agent?

**Decision:** gunakan controlled workflow dengan bounded AI components.

### Alasan

Autonomous agent memberikan:

* unpredictable tool calls;
* larger blast radius;
* sulit diaudit;
* sulit diprediksi cost;
* lebih sulit memastikan authorization.

Sedangkan workflow:

```text
classify
→ extract
→ validate
→ resolve
→ propose
→ approve
→ execute
```

lebih:

* predictable;
* testable;
* observable;
* auditable;
* secure.

Agentic behavior bisa ditambahkan **hanya pada bagian yang memang membutuhkan iterative reasoning**, misalnya research.

---

# 19. ADR-002 — Kenapa PostgreSQL?

**Decision:** PostgreSQL sebagai primary datastore.

Karena kita membutuhkan:

* relational integrity;
* transactions;
* unique constraints;
* audit references;
* JSONB untuk AI output.

Tidak perlu memakai vector database pada MVP kecuali knowledge/retrieval benar-benar membutuhkan semantic search.

---

# 20. ADR-003 — Human Approval

**Decision:** consequential actions membutuhkan approval.

Contoh:

```text
Generate response       → AI
Draft response          → AI
Send response           → Human approval
CRM recommendation      → AI
Critical CRM mutation   → Human approval
Audit event             → System
```


---

# 21. Vertical Slice yang akan kita demonstrasikan

Daripada membangun semuanya, saya ingin assessment kita fokus pada satu alur yang **sangat representatif**:

```text
Customer Email
      ↓
Ingestion
      ↓
Normalization
      ↓
LLM
 ┌────┴─────────────┐
 │                  │
Classify          Extract
 │                  │
 └────────┬─────────┘
          ↓
     Validation
          ↓
     CRM Resolver
          ↓
    Action Proposal
          ↓
     Policy Engine
          ↓
    Human Approval
          ↓
    CRM / Response
          ↓
      Audit Log
```

---

## Stack awal

| Layer         | Pilihan                                     |
| ------------- | ------------------------------------------- |
| API           | **Go** atau **FastAPI**                     |
| Workflow      | Application service + queue                 |
| Database      | **PostgreSQL**                              |
| Cache/queue   | Redis, bila diperlukan                      |
| AI            | LLM provider abstraction                    |
| Validation    | JSON Schema / typed models                  |
| Auth          | OAuth/API keys + scoped service credentials |
| Secrets       | Environment/secret manager                  |
| Observability | Structured logs + metrics                   |
| Audit         | PostgreSQL append-only audit events         |
| Deployment    | Containerized                               |
