# PRD — BEDA AI Intake & CRM Automation System

**Status:** Draft v0.1
**Tujuan:** Technical Assessment BEDA AI Internship
**Target:** Internal BEDA
**Tanggal:** 31 Agustus 2026

---

## 1. Ringkasan Produk

### Nama sementara

**BEDA AI Intake & CRM Automation**

### Deskripsi

Sistem yang menerima pertanyaan/inquiry bisnis dari berbagai channel seperti:

* Email
* Website form
* Messaging channel

Kemudian sistem secara otomatis:

1. menerima dan menormalisasi data,
2. mengidentifikasi jenis inquiry,
3. mengekstrak informasi penting,
4. menentukan apakah informasi sudah cukup,
5. melakukan research jika diperlukan,
6. meminta informasi tambahan jika diperlukan,
7. mencari atau membuat record CRM yang sesuai,
8. membuat rekomendasi/draft respons,
9. memberi notifikasi kepada orang yang tepat,
10. menyimpan audit trail setiap keputusan dan tindakan.

Sistem **tidak memiliki kewenangan penuh untuk melakukan tindakan consequential** tanpa persetujuan manusia.

---

# 2. Problem Statement

BEDA menerima berbagai inquiry dari beberapa channel dengan format dan kualitas informasi yang berbeda.

Contohnya:

### Inquiry A

> Hi, I'm interested in working with BEDA. We have a team of 20 people and want to improve our sales process. Can someone contact me?

Informasinya relatif lengkap.

### Inquiry B

> How much does your service cost?

Hanya memiliki sedikit informasi.

### Inquiry C

> Hi, I want to know more about your company.

Belum jelas apakah ini:

* calon customer,
* partner,
* job seeker,
* general inquiry.

### Inquiry D

> Buy followers now!!!

Spam.

Masalahnya adalah **informasi tidak terstruktur dan volume inquiry dapat meningkat**, sehingga manusia harus menghabiskan waktu untuk:

* membaca,
* mengklasifikasikan,
* mencari informasi,
* memasukkan data ke CRM,
* menentukan siapa yang harus menangani,
* dan membuat respons.

---

# 3. Goals

### Primary Goals

Sistem harus mampu:

* menerima inquiry dari berbagai sumber;
* mengubah input tidak terstruktur menjadi data terstruktur;
* mengklasifikasikan inquiry;
* mengekstrak informasi bisnis yang relevan;
* mendeteksi informasi yang hilang;
* membantu melakukan research;
* menemukan record CRM yang sudah ada;
* mencegah duplicate CRM record;
* membuat atau memperbarui CRM record secara aman;
* membuat draft respons;
* melakukan routing kepada manusia yang tepat;
* menyimpan audit trail yang dapat dipercaya;
* tetap aman ketika LLM/API gagal.

### Success Criteria

Secara konseptual:

> **Human effort berkurang tanpa memberikan authority yang tidak terkontrol kepada AI.**

---

# 4. Non-Goals

Untuk MVP, sistem **tidak bertujuan** menjadi:

* autonomous sales agent penuh;
* autonomous customer support agent;
* pengganti CRM;
* general-purpose AI agent;
* sistem yang bebas menjalankan arbitrary tools;
* sistem yang otomatis mengirim semua pesan kepada customer.

Khususnya:

> **LLM tidak boleh memiliki akses langsung tanpa pembatasan terhadap CRM, email, atau sistem eksternal.**

---

# 5. User / Actor

### 5.1 Incoming Customer / Prospect

Mengirim inquiry melalui:

* email,
* website,
* messaging.

Tidak berinteraksi langsung dengan AI orchestration layer.

### 5.2 Operations / Sales Team

Menerima:

* classified inquiry,
* extracted information,
* recommended action,
* draft response,
* missing information.

### 5.3 Manager / Authorized Approver

Memberikan approval untuk tindakan consequential.

Contoh:

* mengirim komunikasi eksternal tertentu,
* mengubah data penting,
* melakukan tindakan komersial.

### 5.4 AI System

Bertugas sebagai:

> **analysis + recommendation + drafting system**

bukan sebagai pemegang authority bisnis.

### 5.5 System Administrator

Mengelola:

* integrations,
* permissions,
* secrets,
* routing rules,
* model configuration,
* audit access.

---

# 6. High-Level User Flow

```text
Incoming Inquiry
       │
       ▼
┌──────────────────┐
│     Ingestion    │
│ Email / Form /   │
│ Messaging        │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│   Normalization  │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ AI Classification│
│ + Extraction     │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Validation &     │
│ Confidence Check │
└────────┬─────────┘
         │
    ┌────┴─────┐
    │           │
    ▼           ▼
Complete     Incomplete
    │           │
    │           ▼
    │      Research / Ask
    │      for information
    │
    └──────┬────┘
           ▼
┌──────────────────┐
│ CRM Resolution   │
│ Find/Create/     │
│ Update           │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Policy /         │
│ Approval Gate    │
└────────┬─────────┘
         │
    ┌────┴─────┐
    │           │
    ▼           ▼
 Allowed      Approval
 Action       Required
    │           │
    │           ▼
    │       Human Review
    │           │
    └─────┬─────┘
          ▼
┌──────────────────┐
│ Draft / Routing  │
│ / Notification   │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│   Audit Trail    │
└──────────────────┘
```

---

# 7. Functional Requirements

## FR-01 — Multi-Channel Ingestion

Sistem harus dapat menerima inquiry dari beberapa channel.

**Input minimal:**

```text
source
external_message_id
received_at
sender
subject
content
attachments
metadata
```

Sistem harus mempertahankan `external_message_id` untuk idempotency.

---

## FR-02 — Normalization

Sistem mengubah berbagai format input menjadi canonical representation.

Contoh:

```json
{
  "id": "inq_123",
  "source": "email",
  "sender": {
    "name": "John Doe",
    "email": "john@example.com"
  },
  "subject": "Interested in BEDA",
  "content": "...",
  "received_at": "..."
}
```

Tujuannya agar downstream system tidak perlu memahami format setiap channel.

---

# 8. FR-03 — Classification

Sistem harus mengklasifikasikan inquiry.

Kategori awal:

```text
sales
support
partnership
career
general
spam
unknown
```

LLM boleh digunakan untuk memahami konteks.

Tetapi hasilnya harus melalui:

* schema validation,
* confidence evaluation,
* deterministic policy.

---

# 9. FR-04 — Information Extraction

Sistem mengekstrak informasi yang relevan.

Contoh:

```json
{
  "person": {
    "name": "...",
    "email": "..."
  },
  "company": {
    "name": "...",
    "website": "..."
  },
  "intent": "...",
  "budget": null,
  "team_size": 20,
  "timeline": null
}
```

**Penting:**

Field yang tidak diketahui harus menjadi:

```text
null
```

bukan ditebak oleh model.

---

# 10. FR-05 — Missing Information Detection

Sistem menentukan apakah inquiry memiliki informasi yang cukup.

Contoh:

```text
Required for sales qualification:

company_name
contact
business_need
timeline
```

Jika:

```text
company_name = null
timeline = null
```

sistem tidak boleh mengarang nilai.

Status:

```text
NEEDS_INFORMATION
```

Kemudian sistem dapat membuat:

> **recommended questions**

untuk manusia atau draft follow-up.

---

# 11. FR-06 — Research

Jika dibutuhkan, sistem dapat melakukan research terhadap informasi publik.

Contoh:

```text
Company name
      ↓
Search / public sources
      ↓
Company website
      ↓
Structured findings
```

Research harus menghasilkan:

* source,
* retrieved_at,
* extracted information,
* confidence.

**Research tidak boleh menjadi alasan untuk menganggap informasi yang belum terverifikasi sebagai fakta.**

---

# 12. FR-07 — CRM Resolution

Sistem mencari apakah contact/company sudah ada.

Urutan:

```text
Exact identifier
      ↓
Email
      ↓
Company domain
      ↓
Other matching signals
      ↓
Potential duplicate
```

Hasil:

```text
MATCH
NEW
AMBIGUOUS
```

Jika ambiguous:

> **Human review**

Bukan membuat record baru secara otomatis.

---

# 13. FR-08 — CRM Mutation

Sistem dapat **mengusulkan**:

```text
CREATE
UPDATE
NO_CHANGE
```

Tetapi mutation harus mengikuti policy.

Contoh:

```text
Low-risk metadata update
        ↓
May be automated

Important CRM field
        ↓
Approval required
```

---

# 14. FR-09 — Response Drafting

LLM dapat membuat draft respons berdasarkan:

* inquiry,
* extracted information,
* verified company information,
* approved business context,
* response policy.

Contoh:

```text
Draft generated
       ↓
Policy check
       ↓
Human review
       ↓
Send
```

Untuk komunikasi eksternal yang consequential:

> **AI hanya membuat draft.**

---

# 15. FR-10 — Routing & Notification

Sistem menentukan siapa yang perlu menangani inquiry.

Contoh:

```text
Sales       → Sales team
Support     → Support team
Partnership → Business development
Career      → Recruitment
Unknown     → Operations
```

Jika confidence terlalu rendah:

```text
Unknown
   ↓
Human triage
```

---

# 16. FR-11 — Audit Trail

Setiap event penting harus dicatat.

Contoh:

```json
{
  "event_id": "evt_123",
  "timestamp": "...",
  "actor": "ai_classifier",
  "action": "classify_inquiry",
  "input_reference": "inq_123",
  "result": "sales",
  "model": "model-name",
  "confidence": 0.91,
  "policy_result": "allowed",
  "correlation_id": "..."
}
```

Audit log harus memungkinkan kita menjawab:

> **Siapa melakukan apa, kapan, berdasarkan data apa, dan hasilnya apa?**

---

# 17. FR-12 — Human Approval

Sistem harus memiliki approval state.

Contoh:

```text
PENDING
APPROVED
REJECTED
EXPIRED
```

Tidak boleh:

```text
LLM → send email
```

Melainkan:

```text
LLM
 ↓
Draft
 ↓
Policy Engine
 ↓
Approval Required
 ↓
Human
 ↓
Approved
 ↓
Deterministic Action
```

---

# 18. AI vs Deterministic Logic

Ini sebaiknya **menjadi prinsip utama PRD**.

| Fungsi                  | AI/LLM |     Deterministic |
| ----------------------- | -----: | ----------------: |
| Memahami inquiry        |      ✅ |                   |
| Classification          |      ✅ |        validation |
| Information extraction  |      ✅ | schema validation |
| Summarization           |      ✅ |                   |
| Draft response          |      ✅ |            policy |
| Research interpretation |      ✅ |   source tracking |
| Required fields         |        |                 ✅ |
| Deduplication           |        |                 ✅ |
| Permissions             |        |                 ✅ |
| Authentication          |        |                 ✅ |
| Authorization           |        |                 ✅ |
| CRM mutation policy     |        |                 ✅ |
| Approval                |        |                 ✅ |
| Retry                   |        |                 ✅ |
| Idempotency             |        |                 ✅ |
| Audit logging           |        |                 ✅ |
| Secret management       |        |                 ✅ |
| Tool access             |        |                 ✅ |

**Prinsip:**

> **LLM menentukan/merekomendasikan “apa yang mungkin terjadi”; deterministic application menentukan “apa yang boleh terjadi”.**

Ini menurut saya bisa menjadi salah satu kalimat utama submission kita.

---

# 19. Error & Failure Handling

## LLM Failure

Jika timeout/error:

```text
Retry
 ↓
Fallback model / strategy
 ↓
Human review
```

Tidak boleh melakukan consequential action hanya karena model gagal.

### Invalid Output

Jika LLM menghasilkan:

```json
{
  "category": "maybe-sales"
}
```

schema validation menolak output.

---

## Hallucination

Mitigasi:

* structured output;
* schema validation;
* explicit `unknown/null`;
* source attribution;
* retrieval untuk factual information;
* confidence threshold;
* human approval.

Prinsip:

> **Missing information harus tetap missing, bukan diisi dengan tebakan.**

---

## Duplicate Inquiry

Gunakan:

```text
source + external_message_id
```

untuk idempotency.

Untuk CRM:

```text
email
company domain
CRM identifiers
```

Jika ambiguous:

> Human review.

---

## API Failure

Gunakan:

* timeout;
* retry dengan exponential backoff;
* idempotency key;
* circuit breaker bila diperlukan;
* dead-letter queue untuk event yang gagal diproses;
* observability.

---

# 20. Security Requirements

### Least Privilege

LLM tidak mendapatkan:

```text
full CRM access
full database access
arbitrary HTTP access
shell access
```

Tool harus memiliki permission yang scoped.

Contoh:

```text
crm.read_contact
crm.create_draft
crm.propose_update
```

bukan:

```text
crm.admin
```

---

### Secrets

API keys dan credentials:

* tidak disimpan dalam prompt;
* tidak di-hardcode;
* tidak dimasukkan ke repository;
* menggunakan secret manager/environment-secured configuration.

---

### Sensitive Data

Data bisnis harus:

* encrypted in transit;
* encrypted at rest bila storage mendukung;
* memiliki access control;
* memiliki retention policy;
* tercatat dalam audit trail untuk akses penting.

---

# 21. Cost & Latency

Tidak semua inquiry perlu menggunakan LLM besar.

Contoh:

```text
Known spam
   ↓
Deterministic filter
   ↓
Reject
```

Tidak perlu LLM.

Sedangkan:

```text
Complex business inquiry
   ↓
LLM
```

gunakan model yang sesuai.

Strategi:

* cheap model untuk classification;
* stronger model untuk complex reasoning;
* caching untuk research;
* batched operations jika memungkinkan;
* token limits;
* structured outputs;
* jangan menjalankan agent loop tanpa batas.

Target prinsip:

> **Use the cheapest reliable mechanism for each task.**

---

# 22. Observability

Sistem harus memonitor:

### Metrics

```text
inquiries_received
classification_success_rate
classification_latency
llm_error_rate
crm_duplicate_rate
human_approval_rate
automation_rate
cost_per_inquiry
```

### Logs

Setiap workflow memiliki:

```text
correlation_id
inquiry_id
event_id
```

sehingga satu inquiry dapat ditelusuri end-to-end.

---

# 23. Human-in-the-Loop Policy

### Bisa otomatis

Contoh:

* normalization;
* classification;
* extraction;
* spam filtering;
* internal routing;
* generating drafts.

### Membutuhkan approval

Contoh:

* mengirim komunikasi eksternal tertentu;
* mengubah data CRM kritis;
* membuat commercial commitment;
* tindakan yang berdampak pada customer/business;
* ambiguous CRM merge.

---

# 24. Automation Boundary

### Satu hal yang sengaja **tidak** kita otomatisasi:

> **Pengiriman komunikasi eksternal yang memiliki konsekuensi komersial, hukum, atau reputasi.**

AI boleh:

```text
understand
→ research
→ recommend
→ draft
```

Tetapi:

```text
SEND
```

tetap membutuhkan manusia ketika consequential.

---

# 25. MVP Scope

Untuk technical assessment, kita **tidak perlu membangun seluruh production system**.

MVP konseptual:

```text
Input
 ↓
Normalize
 ↓
LLM Classification + Extraction
 ↓
Validation
 ↓
CRM Resolution
 ↓
Policy Engine
 ↓
Human Approval
 ↓
Draft Response
 ↓
Audit Log
```

Yang penting kita bisa menunjukkan **satu vertical slice yang benar-benar masuk akal**.

Misalnya:

> **Email inquiry → classification → extraction → duplicate check → CRM proposal → approval → audit**

Itu sudah cukup untuk menunjukkan kemampuan systems thinking.

---

# 26. Acceptance Criteria

### AC-01

Jika inquiry masuk dua kali dengan `external_message_id` yang sama:

> sistem tidak membuat duplicate processing.

### AC-02

Jika LLM tidak yakin terhadap klasifikasi:

> inquiry masuk ke human review.

### AC-03

Jika field penting tidak tersedia:

> sistem menyimpan `null`, bukan hallucinated value.

### AC-04

Jika CRM match memiliki lebih dari satu kandidat:

> sistem tidak memilih secara otomatis.

### AC-05

Jika API CRM gagal:

> sistem melakukan retry tanpa menyebabkan duplicate mutation.

### AC-06

Jika response bersifat consequential:

> sistem hanya membuat draft dan meminta approval.

### AC-07

Setiap perubahan state harus:

> menghasilkan audit event.

### AC-08

LLM tidak memiliki arbitrary access ke:

> CRM, database, email, atau external tools.

---

# 27. Prinsip Desain

Saya ingin kita pegang **7 prinsip** ini ketika nanti membuat architecture:

1. **AI suggests, code decides.**
2. **Unknown is better than hallucinated.**
3. **Human approval for consequential actions.**
4. **Every mutation must be traceable.**
5. **Idempotency before automation.**
6. **Least privilege by default.**
7. **Start simple, scale when evidence requires it.**

