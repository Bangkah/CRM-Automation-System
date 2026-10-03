param(
    [ValidateSet("show", "approve", "reject")]
    [string]$Decision = "show",
    [string]$ApiUrl = "http://localhost:8080"
)

$ErrorActionPreference = "Stop"

$body = @{
    source = "email"
    external_message_id = "demo-$([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds())"
    sender = @{
        email = "customer@example.com"
        name = "Demo Customer"
    }
    subject = "Permintaan promosi"
    content = "We are a team of 20 and want to improve our sales process. Please send a promotional message about our new product."
} | ConvertTo-Json -Depth 5

Write-Host "`n=== BEDA WORKFLOW ===" -ForegroundColor Cyan
Write-Host "Mengirim inquiry untuk diklasifikasikan..."

$result = Invoke-RestMethod `
    -Method Post `
    -Uri "$ApiUrl/api/v1/inquiries" `
    -ContentType "application/json" `
    -Body $body

Write-Host "`nKlasifikasi" -ForegroundColor Yellow
Write-Host "  Kategori   : $($result.classification.Category)"
Write-Host "  Confidence : $($result.classification.Confidence)"

Write-Host "`nKeputusan sistem" -ForegroundColor Yellow
Write-Host "  Keputusan  : $($result.policy_decision.Decision)"
Write-Host "  Alasan     : $($result.policy_decision.Reason)"
Write-Host "  Action     : $($result.action.Type)"
Write-Host "  Status     : $($result.action_state)"
Write-Host "  Action ID  : $($result.action_id)"

if ($result.policy_decision.Decision -ne "REQUIRE_APPROVAL") {
    Write-Host "`nTidak ada persetujuan manusia yang diperlukan." -ForegroundColor Green
    exit 0
}

Write-Host "`nStatus: MENUNGGU PERSETUJUAN MANUSIA" -ForegroundColor Magenta

if ($Decision -eq "show") {
    Write-Host "Gunakan salah satu perintah berikut:"
    Write-Host "  .\scripts\demo-workflow.ps1 -Decision approve"
    Write-Host "  .\scripts\demo-workflow.ps1 -Decision reject"
    exit 0
}

$decisionBody = @{
    approver_id = "human-reviewer"
}
if ($Decision -eq "reject") {
    $decisionBody.reason = "Ditolak oleh reviewer"
}

$decisionResult = Invoke-RestMethod `
    -Method Post `
    -Uri "$ApiUrl/api/v1/actions/$($result.action_id)/$Decision" `
    -ContentType "application/json" `
    -Body ($decisionBody | ConvertTo-Json)

Write-Host "`n=== HASIL PERSETUJUAN ===" -ForegroundColor Cyan
Write-Host "  Action ID  : $($decisionResult.action_id)"
Write-Host "  Status     : $($decisionResult.state)"
Write-Host "  Keputusan  : $($decisionResult.decision)"
