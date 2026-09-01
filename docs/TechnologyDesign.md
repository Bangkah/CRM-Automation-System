# Technology Design

## 3.1 Stack yang dipilih

| Komponen          | Pilihan                              | Alasan                                                         |
| ----------------- | ------------------------------------ | -------------------------------------------------------------- |
| API               | **Go**                               | Strong typing, concurrency, cocok untuk backend/infrastructure |
| Worker            | **Go**                               | Satu bahasa untuk API + processing                             |
| Database          | **PostgreSQL**                       | Transaction, constraint, JSONB, reliable                       |
| Queue             | **PostgreSQL-backed jobs** untuk MVP | Mengurangi infrastructure tanpa kehilangan async processing    |
| Cache             | Tidak wajib                          | Tambahkan Redis hanya jika ada kebutuhan nyata                 |
| LLM               | **Provider abstraction**             | Tidak terkunci ke satu vendor                                  |
| Structured output | JSON Schema                          | Output AI dapat divalidasi                                     |
| Research          | Search provider abstraction          | Bisa diganti provider                                          |
| CRM               | CRM adapter/interface                | Tidak coupling ke vendor tertentu                              |
| Auth              | OAuth/API keys + scoped credentials  | Least privilege                                                |
| Secrets           | Environment/secret manager           | Secret tidak masuk prompt                                      |
| Audit             | PostgreSQL append-only events        | Traceability                                                   |
| Deployment        | Docker                               | Reproducible deployment                                        |
| Observability     | Structured logs + metrics            | Debugging & operations                                         |

---

# 3.2 LLM Provider Abstraction

Jangan hardcode:

```go
client := OpenAI(...)
```

di seluruh codebase.

Buat interface:

```go
type LLMProvider interface {
    Classify(ctx context.Context, input Inquiry) (Classification, error)
    Extract(ctx context.Context, input Inquiry) (Extraction, error)
    Draft(ctx context.Context, input DraftRequest) (Draft, error)
}
```

Implementasi:

```text
LLMProvider
   ├── OpenAIProvider
   ├── GeminiProvider
   ├── AnthropicProvider
   └── LocalProvider
```

Keuntungannya:

* vendor independence;
* easier testing;
* fallback;
* cost optimization;
* model replacement tanpa rewrite workflow.

---

# 3.3 Model Routing


```text
                    Inquiry
                       │
                       ▼
                 Task Router
                 /          \
                /            \
        Simple task       Complex task
             │                  │
             ▼                  ▼
        Small model        Strong model
```

Contoh:

### Small model

* spam detection;
* basic classification;
* extraction sederhana.

### Strong model

* ambiguous inquiry;
* multi-step reasoning;
* research synthesis;
* complicated response drafting.

### Human

* high-risk decision;
* ambiguous identity;
* consequential action.

---

# 3.4 Research Abstraction

Sama seperti LLM:

```go
type ResearchProvider interface {
    Search(ctx context.Context, query string) ([]Evidence, error)
}
```

Implementasi bisa:

```text
ResearchProvider
    ├── WebSearch
    ├── InternalKnowledge
    └── MockResearch
```

Dan **research output bukan fakta mentah**.

```go
type Evidence struct {
    SourceURL   string
    Title       string
    RetrievedAt time.Time
    Content     string
}
```

LLM hanya melakukan synthesis dari evidence tersebut.

---

# 3.5 CRM Adapter

Kita tidak mau workflow bergantung pada:

```text
Salesforce.CreateContact()
```

di mana-mana.

Buat:

```go
type CRM interface {
    FindContacts(ctx context.Context, query ContactQuery) ([]Contact, error)
    CreateContact(ctx context.Context, contact Contact) (Contact, error)
    UpdateContact(ctx context.Context, id string, patch ContactPatch) error
}
```

Kemudian:

```text
CRM interface
    ├── SalesforceAdapter
    ├── HubSpotAdapter
    └── MockCRM
```

Assessment bisa memakai MockCRM sehingga seluruh workflow dapat didemonstrasikan tanpa credential CRM sungguhan.

---

# 4. Data Model

Sekarang kita desain database.

## 4.1 inquiries

```sql
CREATE TABLE inquiries (
    id UUID PRIMARY KEY,
    source TEXT NOT NULL,
    external_message_id TEXT NOT NULL,
    sender_email TEXT,
    subject TEXT,
    content TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (source, external_message_id)
);
```

Constraint ini langsung menangani duplicate event.

---

# 4.2 classifications

```sql
CREATE TABLE classifications (
    id UUID PRIMARY KEY,
    inquiry_id UUID NOT NULL REFERENCES inquiries(id),
    category TEXT NOT NULL,
    confidence NUMERIC(4,3) NOT NULL,
    model TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Misalnya:

```text
sales      0.94
support    0.88
spam       0.99
unknown    0.51
```

---

# 4.3 extracted_data

```sql
CREATE TABLE extracted_data (
    id UUID PRIMARY KEY,
    inquiry_id UUID NOT NULL REFERENCES inquiries(id),
    data JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Contoh:

```json
{
  "company": "Acme",
  "team_size": 20,
  "timeline": null,
  "budget": null
}
```

**null lebih baik daripada hallucinated value.**

---

# 4.4 crm_matches

```sql
CREATE TABLE crm_matches (
    id UUID PRIMARY KEY,
    inquiry_id UUID NOT NULL REFERENCES inquiries(id),
    crm_record_id TEXT,
    match_type TEXT NOT NULL,
    confidence NUMERIC(4,3),
    requires_review BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Contoh:

```text
exact_email
domain_match
ambiguous
new_contact
```

---

# 4.5 action_proposals

Ini tabel penting.

```sql
CREATE TABLE action_proposals (
    id UUID PRIMARY KEY,
    inquiry_id UUID NOT NULL REFERENCES inquiries(id),
    action_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    risk_level TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Status:

```text
proposed
approved
rejected
executed
failed
blocked
```

---

# 4.6 approvals

```sql
CREATE TABLE approvals (
    id UUID PRIMARY KEY,
    action_id UUID NOT NULL REFERENCES action_proposals(id),
    reviewer_id TEXT NOT NULL,
    decision TEXT NOT NULL,
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Penting:

**approval merupakan event terpisah dari action.**

Jadi kita bisa mengetahui:

```text
AI proposed
     ↓
Human approved
     ↓
System executed
```

---

# 4.7 audit_events

```sql
CREATE TABLE audit_events (
    id UUID PRIMARY KEY,
    inquiry_id UUID,
    action_id UUID,
    actor_type TEXT NOT NULL,
    actor_id TEXT,
    event_type TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Contoh event:

```text
inquiry.received
ai.classified
ai.extracted
crm.match_found
action.proposed
approval.requested
approval.granted
crm.updated
response.sent
action.failed
```

---

# 5. ERD

Secara sederhana:

```text
                    ┌─────────────────┐
                    │    inquiries    │
                    │─────────────────│
                    │ id PK           │
                    │ source          │
                    │ external_id     │
                    │ content         │
                    └───────┬─────────┘
                            │
            ┌───────────────┼────────────────┐
            │               │                │
            ▼               ▼                ▼
 ┌────────────────┐ ┌──────────────┐ ┌───────────────┐
 │ classifications│ │ extracted    │ │ crm_matches   │
 │────────────────│ │ data         │ │───────────────│
 │ inquiry_id FK  │ │──────────────│ │ inquiry_id FK │
 │ category       │ │ inquiry_id   │ │ record_id     │
 │ confidence     │ │ data JSONB   │ │ match_type    │
 └────────────────┘ └──────────────┘ └───────────────┘
                            │
                            ▼
                   ┌──────────────────┐
                   │ action_proposals │
                   │──────────────────│
                   │ inquiry_id       │
                   │ action_type      │
                   │ payload          │
                   │ risk_level       │
                   │ status           │
                   └────────┬─────────┘
                            │
                            ▼
                     ┌────────────┐
                     │ approvals  │
                     │────────────│
                     │ action_id  │
                     │ reviewer   │
                     │ decision   │
                     └────────────┘

                   ┌────────────────┐
                   │ audit_events   │
                   │────────────────│
                   │ inquiry_id     │
                   │ action_id      │
                   │ actor          │
                   │ event_type     │
                   └────────────────┘
```

---

# 6. State Machine

Kita juga perlu mendefinisikan lifecycle inquiry.

```text
                    ┌───────────┐
                    │ RECEIVED  │
                    └─────┬─────┘
                          ▼
                    ┌───────────┐
                    │ ANALYZING │
                    └─────┬─────┘
                          ▼
                   ┌──────────────┐
                   │   VALIDATED  │
                   └──────┬───────┘
                          │
              ┌───────────┼────────────┐
              ▼           ▼            ▼
          COMPLETE      MISSING      LOW_CONF
              │           │            │
              │           ▼            ▼
              │       CLARIFY       REVIEW
              │           │            │
              └───────────┼────────────┘
                          ▼
                     CRM_RESOLVE
                          │
                  ┌───────┴────────┐
                  ▼                ▼
               MATCHED          AMBIGUOUS
                  │                │
                  │                ▼
                  │             REVIEW
                  │
                  ▼
             ACTION_PROPOSED
                  │
           ┌──────┴───────┐
           ▼              ▼
        LOW_RISK       HIGH_RISK
           │              │
           │              ▼
           │           APPROVAL
           │              │
           │         ┌────┴────┐
           │         ▼         ▼
           │      APPROVED   REJECTED
           │         │
           └─────────┤
                     ▼
                  EXECUTED
                     │
                     ▼
                  AUDITED
```

Ini menunjukkan bahwa sistem **bukan sekadar chatbot**.

Ini adalah workflow engine dengan AI di beberapa titik.

---

# 7. Sequence Diagram — Happy Path

Contoh customer mengirim email sales.

```text
Customer
   │
   │ email
   ▼
Ingestion API
   │
   │ create inquiry
   ▼
PostgreSQL
   │
   │ job
   ▼
AI Worker
   │
   ├── classify
   ├── extract
   │
   ▼
Validator
   │
   ▼
CRM Resolver
   │
   │ exact email match
   ▼
Policy Engine
   │
   │ action = update metadata
   │ low risk
   ▼
Action Executor
   │
   ▼
CRM
   │
   ▼
Audit Service
```

---

# 8. Sequence Diagram — High Risk

Misalnya AI ingin mengirim response ke external customer.

```text
Customer
   │
   ▼
Inquiry
   │
   ▼
LLM
   │
   │ draft response
   ▼
Validator
   │
   ▼
Policy Engine
   │
   │ HIGH RISK
   ▼
Approval Queue
   │
   ▼
Human
   │
   │ approve
   ▼
Action Executor
   │
   ▼
Email Provider
   │
   ▼
Audit
```

Yang penting:

**LLM tidak pernah memiliki jalur langsung ke Email Provider.**

---

# 9. Pseudocode Core Workflow

Ini kemungkinan akan kita masukkan ke technical submission:

```text
processInquiry(inquiry):

    if alreadyProcessed(inquiry.source, inquiry.externalID):
        return

    normalized = normalize(inquiry)

    classification = llm.classify(normalized)
    validate(classification)

    extracted = llm.extract(normalized)
    validate(extracted)

    if classification.confidence < MIN_CONFIDENCE:
        createHumanReview(inquiry, "low_confidence")
        return

    if hasMissingRequiredData(extracted):
        requestMissingInformation(inquiry)
        return

    match = crm.resolve(extracted)

    if match.isAmbiguous:
        createHumanReview(inquiry, "ambiguous_crm_match")
        return

    proposal = createActionProposal(
        inquiry,
        match,
        extracted
    )

    policy = policyEngine.evaluate(proposal)

    if policy.denied:
        audit("action.blocked")
        return

    if policy.requiresApproval:
        requestApproval(proposal)
        return

    executeIdempotently(proposal)

    audit("action.executed")
```

Perhatikan sesuatu yang penting:

**AI hanya muncul di `classify()` dan `extract()`** dalam core path.

Bukan:

```text
agent.doEverything()
```

---

# 10. Kenapa desain ini cocok dengan challenge mereka?

Karena tujuh pertanyaan mereka terjawab secara langsung:

### 1. Architecture

Ada ingestion → AI → validation → CRM → policy → action → audit.

### 2. Model & tools

Ada provider abstraction + model routing.

### 3. LLM vs deterministic code

**LLM:**

* classification;
* extraction;
* reasoning;
* drafting;
* research synthesis.

**Deterministic:**

* authentication;
* validation;
* authorization;
* CRM matching;
* idempotency;
* policy;
* execution;
* audit;
* retry.

### 4. Missing / hallucination / duplicate / failure

Semua sudah punya mekanisme eksplisit.

### 5. Permissions / secrets

Scoped credentials + policy engine + secret isolation.

### 6. Cost / latency

Small model untuk task sederhana + queue + timeout + token budget.

### 7. One thing intentionally not automated

Kita bisa pilih:

> **Sending consequential external communications without human approval.**

Ini sangat defensible.

---

# 11. Satu keputusan yang saya sarankan kita buat

Untuk challenge ini, **jangan membuat repository besar**.

Buat repo kecil seperti:

```text
beda-ai-intake/
├── README.md
├── docs/
│   ├── PRD.md
│   ├── ARCHITECTURE.md
│   ├── SECURITY.md
│   ├── ADR.md
│   └── THREAT-MODEL.md
│
├── cmd/
│   └── api/
│
├── internal/
│   ├── ingestion/
│   ├── ai/
│   ├── validation/
│   ├── crm/
│   ├── policy/
│   ├── approval/
│   └── audit/
│
├── migrations/
│
├── docker-compose.yml
└── README.md
```

Dan **vertical slice yang benar-benar jalan**:

```text
POST /inquiries
       ↓
classification
       ↓
extraction
       ↓
validation
       ↓
mock CRM resolution
       ↓
policy
       ↓
action proposal
       ↓
audit
```

Dengan satu demo test:

```text
"Hi, I'm from Acme and we're interested
in automating our customer support workflow."
```

→ classification `sales`

→ extraction `company=Acme`

→ CRM lookup

→ action proposal

→ policy evaluation

→ audit event.

