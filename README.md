# BEDA AI Inquiry Processing System

Prototype workflow untuk menerima, mengklasifikasikan, dan memproses inquiry bisnis
dengan batas persetujuan manusia yang eksplisit.

## Alur singkat

```text
inquiry
  -> classification dan extraction
  -> validation
  -> CRM resolution
  -> policy decision
  -> approval (jika diperlukan)
  -> execution
  -> audit trail
```

Prinsip utamanya:

> Model memberikan rekomendasi. Aplikasi yang mengambil keputusan.

Provider AI dan CRM pada prototype ini masih mock. Validasi, policy, transisi approval,
idempotency, dan audit trail ditangani secara deterministik oleh aplikasi.

## Prasyarat

- Go 1.22 atau lebih baru
- PowerShell 5.1 atau PowerShell 7 untuk menjalankan demo Windows

## Menjalankan secara lokal

Dari root repository:

```powershell
go test ./...
go run .\cmd\api
```

API berjalan di `http://localhost:8080`. Biarkan terminal ini tetap berjalan, lalu
buka terminal PowerShell kedua untuk menjalankan demo.

## Demo klasifikasi dan persetujuan manusia

Perintah berikut mengirim inquiry contoh dan menampilkan hasil dalam format ringkas:

```powershell
.\scripts\demo-workflow.ps1
```

Contoh output:

```text
=== BEDA WORKFLOW ===
Mengirim inquiry untuk diklasifikasikan...

Klasifikasi
  Kategori   : sales
  Confidence : 0.94

Keputusan sistem
  Keputusan  : REQUIRE_APPROVAL
  Alasan     : external communication requires approval
  Action     : send_external_message
  Status     : PENDING_APPROVAL
  Action ID  : act-...

Status: MENUNGGU PERSETUJUAN MANUSIA
```

Untuk menjalankan demo sekaligus menyetujui action:

```powershell
.\scripts\demo-workflow.ps1 -Decision approve
```

Untuk menjalankan demo sekaligus menolak action:

```powershell
.\scripts\demo-workflow.ps1 -Decision reject
```

Setiap eksekusi membuat inquiry dan `action_id` baru. Jika API berjalan pada alamat
berbeda, gunakan parameter `-ApiUrl`:

```powershell
.\scripts\demo-workflow.ps1 -ApiUrl "http://localhost:9090" -Decision approve
```

## Endpoint API

### Membuat inquiry

`POST /api/v1/inquiries`

Contoh PowerShell:

```powershell
$body = @{
    source = "email"
    external_message_id = "msg-123"
    sender = @{
        email = "customer@example.com"
        name = "Customer"
    }
    subject = "Interested in sales automation"
    content = "We are a team of 20 and want to improve our sales process."
} | ConvertTo-Json -Depth 5

Invoke-RestMethod `
    -Method Post `
    -Uri "http://localhost:8080/api/v1/inquiries" `
    -ContentType "application/json" `
    -Body $body | ConvertTo-Json -Depth 10
```

Response utama berisi:

| Field | Keterangan |
| --- | --- |
| `classification` | Kategori dan confidence dari mock AI |
| `policy_decision` | `ALLOW`, `REQUIRE_APPROVAL`, atau `DENY` |
| `action_id` | ID action untuk approval atau rejection |
| `action_state` | Status action saat ini |
| `audit_trail` | Riwayat event workflow |

### Menyetujui action

`POST /api/v1/actions/{ACTION_ID}/approve`

```powershell
$approvalBody = @{ approver_id = "human-reviewer" } | ConvertTo-Json
$actionId = "act-isi-dengan-id-dari-response"

Invoke-RestMethod `
    -Method Post `
    -Uri "http://localhost:8080/api/v1/actions/$actionId/approve" `
    -ContentType "application/json" `
    -Body $approvalBody | ConvertTo-Json
```

Approval menghasilkan status `APPROVED` dan keputusan `ALLOW`.

### Menolak action

`POST /api/v1/actions/{ACTION_ID}/reject`

```powershell
$rejectionBody = @{
    approver_id = "human-reviewer"
    reason = "Tidak disetujui oleh reviewer"
} | ConvertTo-Json
$actionId = "act-isi-dengan-id-dari-response"

Invoke-RestMethod `
    -Method Post `
    -Uri "http://localhost:8080/api/v1/actions/$actionId/reject" `
    -ContentType "application/json" `
    -Body $rejectionBody | ConvertTo-Json
```

Rejection menghasilkan status `REJECTED` dan keputusan `DENY`.

## Status dan keputusan

| Keputusan | Makna |
| --- | --- |
| `ALLOW` | Action boleh diproses oleh workflow |
| `REQUIRE_APPROVAL` | Workflow berhenti sampai reviewer menyetujui atau menolak |
| `DENY` | Action diblokir oleh policy |

Status action yang umum:
`PROPOSED`, `PENDING_APPROVAL`, `APPROVED`, `REJECTED`, `DENIED`, dan `EXECUTED`.

## Struktur proyek

```text
cmd/api/                 Entry point HTTP server
internal/api/            HTTP handler dan request validation
internal/workflow/       Orkestrasi, policy, approval, dan mock provider
scripts/demo-workflow.ps1 Demo klasifikasi dan approval
docs/                    Dokumen desain dan kontrak API
```

## Batasan prototype

- Provider AI dan CRM masih mock.
- Repository masih in-memory dan data hilang saat aplikasi berhenti.
- Belum ada authentication atau RBAC production.
- Belum ada database durable.
- Integrasi eksternal belum tersambung.
- Belum ditujukan untuk deployment production.
