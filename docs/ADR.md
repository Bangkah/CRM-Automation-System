# ADR.md — BEDA AI Intake System

## ADR-001 — Controlled AI Workflow vs Autonomous Agent

**Status:** Accepted

### Context

BEDA menerima inquiry dari berbagai channel dengan kualitas data yang tidak konsisten. Sistem perlu menggunakan AI untuk memahami inquiry, tetapi juga harus mencegah model melakukan tindakan konsekuensial secara mandiri.

### Decision

Kami menggunakan **controlled AI workflow** daripada autonomous agent.

LLM hanya bertanggung jawab atas pekerjaan yang membutuhkan reasoning:

* classification;
* information extraction;
* summarization;
* research synthesis;
* response drafting.

Tindakan yang memengaruhi sistem eksternal tetap dikendalikan oleh deterministic application code.

```text
Input
  ↓
LLM
  ↓
Structured Output
  ↓
Validation
  ↓
Policy
  ↓
Authorization
  ↓
Human Approval (if required)
  ↓
Action Executor
```

### Rationale

Pendekatan ini memberikan:

* predictable behavior;
* smaller blast radius;
* easier testing;
* auditable decisions;
* controlled cost;
* safer integration with CRM and messaging systems.

### Consequence

Sistem mungkin sedikit lebih lambat daripada autonomous agent karena beberapa tindakan membutuhkan approval, tetapi risiko kesalahan yang berdampak pada customer atau CRM jauh lebih rendah.

---

# ADR-002 — LLM sebagai Untrusted Component

**Status:** Accepted

### Context

Input berasal dari pihak eksternal dan dapat mengandung prompt injection atau instruksi berbahaya.

### Decision

LLM diperlakukan sebagai **untrusted reasoning component**.

Customer content tidak dianggap sebagai system instruction.

LLM tidak memiliki akses langsung ke:

* database credentials;
* CRM credentials;
* email credentials;
* arbitrary tools;
* shell execution.

### Rationale

Bahkan model yang sangat kuat tetap dapat menghasilkan output yang salah atau dipengaruhi oleh malicious input.

Security boundary harus berada di luar model.

### Consequence

Setiap model output harus melewati:

```text
Schema Validation
       ↓
Business Validation
       ↓
Policy Engine
       ↓
Authorization
```

---

# ADR-003 — PostgreSQL sebagai Source of Truth

**Status:** Accepted

### Context

Sistem membutuhkan penyimpanan inquiry, extracted data, CRM matches, actions, approvals, dan audit events.

### Decision

PostgreSQL digunakan sebagai primary datastore.

### Rationale

PostgreSQL menyediakan:

* transactions;
* foreign keys;
* unique constraints;
* JSONB;
* indexing;
* reliable persistence.

Untuk produksi, idempotency akan dipaksakan di storage durabel dengan constraint seperti:

```sql
UNIQUE(source, external_message_id)
```

Pada prototype saat ini, deduplicasi dijalankan di memori oleh workflow service. Jadi tidak ada klaim bahwa implementasi saat ini sudah memiliki constraint SQL di production storage.

### Consequence

MVP tidak membutuhkan banyak database berbeda.

Vector database hanya akan ditambahkan apabila kebutuhan retrieval benar-benar membutuhkannya.

---

# ADR-004 — Idempotent Actions

**Status:** Accepted

### Context

External providers dapat mengirim webhook lebih dari sekali dan API dapat mengalami timeout setelah action sebenarnya berhasil.

### Decision

Setiap event dan consequential action memiliki identifier/idempotency key.

```text
event_id
action_id
idempotency_key
```

### Example

```text
CRM UPDATE
     ↓
action_id = {ACTION_ID}
     ↓
timeout
     ↓
retry {ACTION_ID}
```

CRM adapter harus memastikan retry tidak menghasilkan mutation kedua.

### Rationale

Reliability tidak hanya berarti *retry*, tetapi **retry dengan aman**.

---

# ADR-005 — Human Approval untuk Consequential Actions

**Status:** Accepted

### Context

Beberapa action memiliki konsekuensi eksternal.

Contoh:

* mengirim pesan kepada customer;
* mengubah data penting;
* mengubah billing information;
* menghapus record.

### Decision

Action dikategorikan berdasarkan risk:

```text
LOW
 ↓
automatic execution

MEDIUM
 ↓
policy-specific approval

HIGH
 ↓
human approval required

PROHIBITED
 ↓
always blocked
```

### Intentionally not automated

> **Mengirim komunikasi eksternal yang consequential tanpa human approval.**

AI boleh membuat draft, tetapi tidak memiliki authority untuk mengirimnya sendiri.

---

# ADR-006 — Deterministic CRM Resolution

**Status:** Accepted

### Context

LLM dapat salah memilih customer ketika terdapat beberapa record yang mirip.

### Decision

CRM identity resolution dilakukan menggunakan deterministic rules terlebih dahulu.

Urutan:

```text
Exact email
     ↓
Exact external ID
     ↓
Verified domain + additional signals
     ↓
Ambiguous
     ↓
Human review
```

LLM boleh memberikan kandidat atau reasoning, tetapi **tidak menentukan final identity ketika hasilnya ambiguous**.

### Rationale

False positive CRM mutation lebih berbahaya daripada meminta human review.

---

# ADR-007 — Provider Abstraction

**Status:** Accepted

### Context

AI provider, search provider, dan CRM dapat berubah.

### Decision

Semua external AI/integration menggunakan interface.

Contoh:

```go
type LLMProvider interface {
    Classify(ctx context.Context, input Inquiry) (Classification, error)
    Extract(ctx context.Context, input Inquiry) (Extraction, error)
}
```

### Rationale

Mengurangi vendor lock-in dan mempermudah testing.

Kita dapat menggunakan:

```text
Production
    ↓
Real LLM

Development
    ↓
Mock LLM

Testing
    ↓
Deterministic fixture
```

---

# ADR-008 — Model Routing berdasarkan Task

**Status:** Accepted

### Context

Tidak semua task membutuhkan model paling mahal atau paling capable.

### Decision

Model dipilih berdasarkan complexity.

```text
                    Task Router
                       │
            ┌──────────┴──────────┐
            ▼                     ▼
       Simple task           Complex task
            │                     │
            ▼                     ▼
       Small model          Strong model
```

### Simple

* spam classification;
* basic categorization;
* simple extraction.

### Complex

* ambiguous reasoning;
* research synthesis;
* complex response drafting.

### Rationale

Mengurangi:

* token usage;
* latency;
* operational cost.

---

# ADR-009 — Evidence-backed Research

**Status:** Accepted

### Context

AI dapat hallucinate ketika melakukan research.

### Decision

Research results harus menyimpan evidence.

```json
{
  "claim": "...",
  "source": "...",
  "retrieved_at": "...",
  "confidence": 0.91
}
```

LLM hanya melakukan synthesis terhadap evidence yang tersedia.

### Rationale

Sistem dapat membedakan:

```text
VERIFIED
INFERRED
UNKNOWN
```

dan tidak mengubah `UNKNOWN` menjadi fakta.

---

# ADR-010 — Append-oriented Audit Trail

**Status:** Accepted

### Context

BEDA membutuhkan reliable audit trail untuk mengetahui apa yang dilakukan AI, system, dan manusia.

### Decision

Setiap significant event disimpan sebagai audit event.

Contoh:

```text
inquiry.received
ai.classified
ai.extracted
crm.match_found
action.proposed
approval.requested
approval.granted
action.executed
action.failed
```

Audit record tidak digunakan sebagai mutable application state.

### Rationale

Hal ini memungkinkan reconstruction:

```text
"What happened to inquiry X?"
```

tanpa harus menebak dari current CRM state.

---

# ADR-011 — Fail Closed untuk Security Decisions

**Status:** Accepted

Ini penting.

Kalau Policy Engine error:

```text
Policy Engine
     ↓
ERROR
```

kita **tidak** melakukan:

```text
ERROR → execute anyway
```

Tetapi:

```text
ERROR
 ↓
BLOCK / REVIEW
 ↓
ALERT
```

Prinsip:

> **When authorization cannot be established, the action is not executed.**

---

# ADR-012 — PostgreSQL-backed Queue untuk MVP

**Status:** Accepted

### Context

Workflow membutuhkan asynchronous processing di lingkungan produksi, tetapi assessment prototype tidak mengimplementasikan infra ini.

### Decision

Ini adalah desain produksi masa depan, bukan current implementation prototype. Current prototype menggunakan synchronous HTTP workflow dengan in-memory repositories dan deduplication.

```text
API
 ↓
future jobs table
 ↓
future worker
```

Redis/SQS/etc. dapat ditambahkan ketika throughput membutuhkan dedicated queue.

### Rationale

Ini mengurangi infrastructure complexity sambil tetap memberikan:

* retry;
* worker processing;
* failure state;
* basic durability.

Namun, repo saat ini tidak menyertakan queue, worker, atau job table yang aktif.

---

# ADR Summary

| Decision            | Pilihan                |
| ------------------- | ---------------------- |
| AI architecture     | Controlled workflow    |
| LLM authority       | None                   |
| Primary DB          | PostgreSQL             |
| Queue               | PostgreSQL-backed jobs |
| CRM                 | Adapter                |
| LLM                 | Provider abstraction   |
| Research            | Evidence-backed        |
| Identity resolution | Deterministic          |
| External actions    | Policy controlled      |
| High-risk action    | Human approval         |
| Secrets             | Never exposed to LLM   |
| Audit               | Append-oriented        |
| Failure security    | Fail closed            |
| Model selection     | Task-based routing     |

---

# Yang paling penting untuk submission

Kalau mereka hanya membaca 30 detik, saya ingin mereka menangkap **empat keputusan ini**:

```text
┌──────────────────────────────────────────────┐
│              BEDA AI SYSTEM                  │
│                                              │
│  1. LLM reasons, but does not have authority │
│                                              │
│  2. Deterministic code controls actions      │
│                                              │
│  3. Ambiguity → human review                 │
│                                              │
│  4. Every consequential action is auditable  │
└──────────────────────────────────────────────┘
```

Itu jauh lebih kuat daripada mengatakan:

> *“Saya akan menggunakan AI agent untuk membaca email dan mengupdate CRM.”*

Karena justru **batasan AI** adalah bagian yang sedang mereka uji.

---

## Berikutnya: kita bikin implementasi minimal

Sekarang dokumen desain kita sudah punya:

* PRD
* Architecture
* Threat Model
* Data Model
* State Machine
* Sequence Flow
* ADR

Tahap berikutnya saya sarankan **langsung membuat struktur repository + API contract + JSON Schema + test cases**.

Targetnya bukan membuat produk 6 bulan dalam satu malam. Targetnya membuat **vertical slice yang bisa mereka clone, jalankan, dan lihat reasoning kamu melalui code**:

```text
POST /inquiries
       ↓
normalize
       ↓
classify
       ↓
extract
       ↓
validate
       ↓
mock CRM resolution
       ↓
policy evaluation
       ↓
action proposal
       ↓
audit
```