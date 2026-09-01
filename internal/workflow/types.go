package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	DecisionAllow = "ALLOW"
	DecisionRequireApproval = "REQUIRE_APPROVAL"
	DecisionDeny = "DENY"
)

const (
	CategorySales = "sales"
	CategorySupport = "support"
	CategorySpam = "spam"
	CategoryOther = "other"
	CategoryUnknown = "unknown"
)

var validCategories = map[string]struct{}{
	CategorySales:   {},
	CategorySupport: {},
	CategorySpam:    {},
	CategoryOther:   {},
	CategoryUnknown: {},
}

var ErrWorkflowFailure = errors.New("workflow provider failure")

// Inquiry is the normalized request from an external channel.
type Inquiry struct {
	ID               string
	Source           string
	ExternalMessageID string
	SenderEmail      string
	Subject          string
	Content          string
	ReceivedAt       time.Time
}

// Classification is the model's constrained categorization.
type Classification struct {
	Category   string
	Confidence float64
}

// Extraction contains structured fields for downstream processing.
type Extraction struct {
	Company     *string
	TeamSize    *int
	Timeline    *string
	Budget      *string
	ContactName *string
}

// CRMMatch captures deterministic identity resolution.
type CRMMatch struct {
	MatchFound bool
	RecordID   string
	Confidence float64
	RequiresReview bool
	MatchType  string
}

// ActionProposal describes the proposed action, including risk.
type ActionProposal struct {
	ID               string
	Type             string
	Description      string
	RequiresApproval bool
	HighRisk         bool
}

// PolicyDecision is the deterministic outcome returned by the policy engine.
type PolicyDecision struct {
	Decision string
	Reason   string
}

// AuditEvent is an immutable record for important workflow transitions.
type AuditEvent struct {
	ID        string
	Type      string
	InquiryID string
	ActionID  string
	Decision  string
	CreatedAt time.Time
	Source    string
}

// WorkflowResult packages a processed inquiry and all derived artifacts.
type WorkflowResult struct {
	ID                 string
	Inquiry            Inquiry
	Classification     Classification
	Extraction         Extraction
	CRMMatch           CRMMatch
	ProposedAction     ActionProposal
	PolicyDecision     PolicyDecision
	AuditTrail         []AuditEvent
	Duplicate          bool
}

// LLMProvider defines the contract for AI classification/extraction.
type LLMProvider interface {
	Classify(ctx context.Context, input Inquiry) (Classification, error)
	Extract(ctx context.Context, input Inquiry) (Extraction, error)
}

// CRMProvider defines the contract for identity resolution.
type CRMProvider interface {
	FindMatch(ctx context.Context, inquiry Inquiry) (CRMMatch, error)
}

type Source struct {
	Provider string
}

// MockLLMProvider is a deterministic test implementation.
type MockLLMProvider struct{}

func (MockLLMProvider) Classify(_ context.Context, input Inquiry) (Classification, error) {
	text := strings.ToLower(input.Content)
	switch {
	case strings.Contains(text, "spam") || strings.Contains(text, "followers") || strings.Contains(text, "buy now"):
		return Classification{Category: CategorySpam, Confidence: 0.99}, nil
	case strings.Contains(text, "support") || strings.Contains(text, "help"):
		return Classification{Category: CategorySupport, Confidence: 0.85}, nil
	case strings.Contains(text, "sales") || strings.Contains(text, "pricing") || strings.Contains(text, "contact us") || strings.Contains(text, "improve our sales process"):
		return Classification{Category: CategorySales, Confidence: 0.94}, nil
	case strings.Contains(text, "delete all crm records") || strings.Contains(text, "api key"):
		return Classification{Category: CategoryOther, Confidence: 0.42}, nil
	default:
		return Classification{Category: CategoryUnknown, Confidence: 0.5}, nil
	}
}

func (MockLLMProvider) Extract(_ context.Context, input Inquiry) (Extraction, error) {
	company := "Acme"
	teamSize := 20
	timeline := "next quarter"
	budget := "unknown"
	contactName := "John"

	text := strings.ToLower(input.Content)
	if strings.Contains(text, "help") && !strings.Contains(text, "sales") && !strings.Contains(text, "pricing") && !strings.Contains(text, "team") {
		return Extraction{Company: nil, TeamSize: nil, Timeline: nil, Budget: nil, ContactName: nil}, nil
	}
	if strings.Contains(text, "delete all crm records") || strings.Contains(text, "api key") {
		return Extraction{Company: nil, TeamSize: nil, Timeline: nil, Budget: nil, ContactName: nil}, nil
	}
	if strings.Contains(text, "we are a team of 20") || strings.Contains(text, "team of 20") {
		return Extraction{Company: &company, TeamSize: &teamSize, Timeline: &timeline, Budget: &budget, ContactName: &contactName}, nil
	}
	if strings.Contains(text, "pricing") {
		return Extraction{Company: &company, TeamSize: &teamSize, Timeline: nil, Budget: &budget, ContactName: &contactName}, nil
	}
	if strings.Contains(text, "send a promotional message") {
		return Extraction{Company: &company, TeamSize: nil, Timeline: nil, Budget: nil, ContactName: &contactName}, nil
	}
	return Extraction{Company: &company, TeamSize: &teamSize, Timeline: &timeline, Budget: &budget, ContactName: &contactName}, nil
}

// MockCRMProvider simulates CRM resolution for the assessment prototype.
type MockCRMProvider struct{}

func (MockCRMProvider) FindMatch(_ context.Context, inquiry Inquiry) (CRMMatch, error) {
	if inquiry.SenderEmail == "john@acme.com" {
		return CRMMatch{MatchFound: true, RecordID: "crm-123", Confidence: 0.97, MatchType: "exact_email"}, nil
	}
	if inquiry.SenderEmail == "hello@example.com" {
		return CRMMatch{MatchFound: false, RecordID: "", Confidence: 0.0, MatchType: "no_match"}, nil
	}
	return CRMMatch{MatchFound: false, RecordID: "", Confidence: 0.0, MatchType: "no_match"}, nil
}

// ValidateClassification ensures provider output follows the constrained schema.
func ValidateClassification(c Classification) error {
	if strings.TrimSpace(c.Category) == "" {
		return errors.New("classification category is required")
	}
	if _, ok := validCategories[c.Category]; !ok {
		return fmt.Errorf("invalid category %q", c.Category)
	}
	if c.Confidence < 0 || c.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1: %f", c.Confidence)
	}
	return nil
}

func actionTypeForInquiry(classification Classification, extraction Extraction) string {
	text := strings.ToLower(strings.TrimSpace(classification.Category))
	if text == CategorySpam {
		return "ignore_spam"
	}
	if strings.Contains(classification.Category, "support") || extraction.Company == nil {
		return "request_clarification"
	}
	if extraction.Company != nil && (extraction.Timeline != nil || extraction.Budget != nil || extraction.TeamSize != nil) {
		return "create_lead"
	}
	return "request_clarification"
}

func determinePolicy(classification Classification, extraction Extraction, crm CRMMatch, actionType string) PolicyDecision {
	if classification.Category == CategorySpam {
		return PolicyDecision{Decision: DecisionDeny, Reason: "spam inquiry is not actionable"}
	}
	if strings.Contains(actionType, "message") || strings.Contains(actionType, "email") || strings.Contains(actionType, "external") {
		return PolicyDecision{Decision: DecisionRequireApproval, Reason: "external communication requires approval"}
	}
	if extraction.Company == nil || extraction.Timeline == nil {
		return PolicyDecision{Decision: DecisionRequireApproval, Reason: "missing critical business details require human review"}
	}
	if crm.MatchFound && crm.Confidence >= 0.9 {
		return PolicyDecision{Decision: DecisionAllow, Reason: "known customer record resolved confidently"}
	}
	if !crm.MatchFound {
		return PolicyDecision{Decision: DecisionRequireApproval, Reason: "customer identity is not yet resolved"}
	}
	return PolicyDecision{Decision: DecisionAllow, Reason: "standard business inquiry"}
}

// ValidationResult represents deterministic validation for LLM output.
type ValidationResult struct {
	Valid     bool
	ErrorText string
}

func ValidateExtraction(extraction Extraction) ValidationResult {
	if extraction.Company != nil && strings.TrimSpace(*extraction.Company) == "" {
		return ValidationResult{Valid: false, ErrorText: "company cannot be blank"}
	}
	if extraction.Timeline != nil && strings.TrimSpace(*extraction.Timeline) == "" {
		return ValidationResult{Valid: false, ErrorText: "timeline cannot be blank"}
	}
	if extraction.Budget != nil && strings.TrimSpace(*extraction.Budget) == "" {
		return ValidationResult{Valid: false, ErrorText: "budget cannot be blank"}
	}
	if extraction.ContactName != nil && strings.TrimSpace(*extraction.ContactName) == "" {
		return ValidationResult{Valid: false, ErrorText: "contact name cannot be blank"}
	}
	return ValidationResult{Valid: true}
}

func createAuditEvent(inquiryID, actionID, eventType, decision, source string) AuditEvent {
	return AuditEvent{
		ID:        fmt.Sprintf("evt-%s-%s", inquiryID, eventType),
		Type:      eventType,
		InquiryID: inquiryID,
		ActionID:  actionID,
		Decision:  decision,
		CreatedAt: time.Now(),
		Source:    source,
	}
}

func normalizeMissingForDecision(extraction Extraction) bool {
	return extraction.Company == nil || extraction.Timeline == nil
}
