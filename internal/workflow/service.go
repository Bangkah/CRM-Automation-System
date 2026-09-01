package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// WorkflowService orchestrates the inquiry pipeline with mock providers.
type WorkflowService struct {
	llm LLMProvider
	crm CRMProvider
	mu   sync.Mutex
	seen map[string]struct{}
}

func NewWorkflowService(llm LLMProvider, crm CRMProvider) *WorkflowService {
	if llm == nil {
		llm = MockLLMProvider{}
	}
	if crm == nil {
		crm = MockCRMProvider{}
	}
	return &WorkflowService{llm: llm, crm: crm, seen: map[string]struct{}{}}
}

func (s *WorkflowService) ProcessInquiry(ctx context.Context, inquiry Inquiry) (WorkflowResult, error) {
	if strings.TrimSpace(inquiry.Source) == "" || strings.TrimSpace(inquiry.Content) == "" {
		return WorkflowResult{}, errors.New("inquiry source and content are required")
	}
	if strings.TrimSpace(inquiry.ExternalMessageID) == "" {
		return WorkflowResult{}, errors.New("external_message_id is required for idempotency")
	}

	key := strings.TrimSpace(inquiry.Source) + ":" + strings.TrimSpace(inquiry.ExternalMessageID)
	s.mu.Lock()
	if _, exists := s.seen[key]; exists {
		s.mu.Unlock()
		result := WorkflowResult{ID: inquiry.ID, Inquiry: inquiry, Duplicate: true, AuditTrail: []AuditEvent{}}
		result.AuditTrail = append(result.AuditTrail, createAuditEvent(inquiry.ID, "", "inquiry.received", "", "ingestion"))
		result.AuditTrail = append(result.AuditTrail, createAuditEvent(inquiry.ID, "", "inquiry.duplicate", "DUPLICATE", "idempotency"))
		return result, nil
	}
	s.seen[key] = struct{}{}
	s.mu.Unlock()

	result := WorkflowResult{ID: inquiry.ID, Inquiry: inquiry, AuditTrail: []AuditEvent{}}
	result.AuditTrail = append(result.AuditTrail, createAuditEvent(inquiry.ID, "", "inquiry.received", "", "ingestion"))

	classification, err := s.llm.Classify(ctx, inquiry)
	if err != nil {
		s.mu.Lock()
		delete(s.seen, key)
		s.mu.Unlock()
		return WorkflowResult{}, fmt.Errorf("classify inquiry: %w", err)
	}
	if err := ValidateClassification(classification); err != nil {
		s.mu.Lock()
		delete(s.seen, key)
		s.mu.Unlock()
		return WorkflowResult{}, fmt.Errorf("validate classification: %w", err)
	}
	result.Classification = classification
	result.AuditTrail = append(result.AuditTrail, createAuditEvent(inquiry.ID, "", "ai.classified", "", "ai"))

	extraction, err := s.llm.Extract(ctx, inquiry)
	if err != nil {
		s.mu.Lock()
		delete(s.seen, key)
		s.mu.Unlock()
		return WorkflowResult{}, fmt.Errorf("extract inquiry: %w", err)
	}
	if validation := ValidateExtraction(extraction); !validation.Valid {
		s.mu.Lock()
		delete(s.seen, key)
		s.mu.Unlock()
		return WorkflowResult{}, fmt.Errorf("validate extraction: %s", validation.ErrorText)
	}
	result.Extraction = extraction
	result.AuditTrail = append(result.AuditTrail, createAuditEvent(inquiry.ID, "", "ai.extracted", "", "ai"))

	if classification.Category == CategorySpam {
		result.PolicyDecision = PolicyDecision{Decision: DecisionDeny, Reason: "spam inquiry"}
		result.AuditTrail = append(result.AuditTrail, createAuditEvent(inquiry.ID, "", "action.blocked", result.PolicyDecision.Decision, "policy"))
		return result, nil
	}

	crmMatch, err := s.crm.FindMatch(ctx, inquiry)
	if err != nil {
		return WorkflowResult{}, fmt.Errorf("resolve crm: %w", err)
	}
	result.CRMMatch = crmMatch
	result.AuditTrail = append(result.AuditTrail, createAuditEvent(inquiry.ID, "", "crm.match_found", result.PolicyDecision.Decision, "crm"))

	proposalType := actionTypeForInquiry(classification, extraction)
	if classification.Category == CategorySupport || extraction.Company == nil {
		proposalType = "clarify_missing_information"
	}
	if strings.Contains(strings.ToLower(inquiry.Content), "send a promotional message") || strings.Contains(strings.ToLower(inquiry.Content), "promotional") {
		proposalType = "send_external_message"
	}
	proposal := ActionProposal{
		ID:               fmt.Sprintf("act-%s", inquiry.ID),
		Type:             proposalType,
		Description:      "Review and potentially route the inquiry",
		RequiresApproval: strings.Contains(proposalType, "message") || strings.Contains(proposalType, "external") || strings.Contains(proposalType, "delete"),
		HighRisk:         strings.Contains(proposalType, "message") || strings.Contains(proposalType, "external"),
	}
	result.ProposedAction = proposal
	result.PolicyDecision = determinePolicy(classification, extraction, crmMatch, proposalType)
	result.AuditTrail = append(result.AuditTrail, createAuditEvent(inquiry.ID, proposal.ID, "action.proposed", result.PolicyDecision.Decision, "workflow"))

	if result.PolicyDecision.Decision == DecisionRequireApproval {
		result.AuditTrail = append(result.AuditTrail, createAuditEvent(inquiry.ID, proposal.ID, "approval.requested", DecisionRequireApproval, "approval"))
	}

	if result.PolicyDecision.Decision == DecisionAllow {
		result.AuditTrail = append(result.AuditTrail, createAuditEvent(inquiry.ID, proposal.ID, "action.executed", DecisionAllow, "workflow"))
	}

	return result, nil
}

func (s *WorkflowService) ProcessInquiryWithDuplicateCheck(ctx context.Context, inquiry Inquiry, seen map[string]struct{}) (WorkflowResult, error) {
	key := inquiry.Source + ":" + inquiry.ExternalMessageID
	if _, ok := seen[key]; ok {
		return WorkflowResult{
			ID:        inquiry.ID,
			Inquiry:   inquiry,
			Duplicate: true,
			AuditTrail: []AuditEvent{createAuditEvent(inquiry.ID, "", "inquiry.duplicate", "DUPLICATE", "idempotency")},
		}, nil
	}
	seen[key] = struct{}{}
	return s.ProcessInquiry(ctx, inquiry)
}

var _ = time.Now
