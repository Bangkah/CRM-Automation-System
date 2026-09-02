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
	llm           LLMProvider
	crm           CRMProvider
	mu            sync.Mutex
	seen          map[string]struct{}
	actionService *ActionService
}

func NewWorkflowService(llm LLMProvider, crm CRMProvider) *WorkflowService {
	if llm == nil {
		llm = MockLLMProvider{}
	}
	if crm == nil {
		crm = MockCRMProvider{}
	}
	repo := newInMemoryActionRepository()
	audit := newInMemoryAuditRepository()
	return &WorkflowService{llm: llm, crm: crm, seen: map[string]struct{}{}, actionService: NewActionService(repo, audit)}
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

	action, err := s.actionService.CreateProposal(ctx, inquiry.ID, proposal.Type, proposal.Description, result.PolicyDecision.Decision, proposal.RequiresApproval, proposal.HighRisk)
	if err != nil {
		return WorkflowResult{}, fmt.Errorf("create action proposal: %w", err)
	}
	result.ActionID = action.ID
	result.ActionState = action.State
	result.AuditTrail = append(result.AuditTrail, createAuditEvent(inquiry.ID, action.ID, "action.proposed", result.PolicyDecision.Decision, "workflow"))

	if result.PolicyDecision.Decision == DecisionDeny {
		if action, err = s.actionService.DenyAction(ctx, action.ID, "policy"); err != nil {
			return WorkflowResult{}, fmt.Errorf("deny action: %w", err)
		}
		result.ActionState = action.State
		result.AuditTrail = append(result.AuditTrail, createAuditEvent(inquiry.ID, action.ID, "action.denied", DecisionDeny, "policy"))
		return result, nil
	}

	if result.PolicyDecision.Decision == DecisionRequireApproval {
		if action, err = s.actionService.RequestApproval(ctx, action.ID, "human-reviewer"); err != nil {
			return WorkflowResult{}, fmt.Errorf("request approval: %w", err)
		}
		result.ActionState = action.State
		result.AuditTrail = append(result.AuditTrail, createAuditEvent(inquiry.ID, action.ID, "approval.requested", DecisionRequireApproval, "approval"))
		return result, nil
	}

	if action, err = s.actionService.ExecuteAction(ctx, action.ID, inquiry.ExternalMessageID); err != nil {
		return WorkflowResult{}, fmt.Errorf("execute action: %w", err)
	}
	result.ActionState = action.State
	result.AuditTrail = append(result.AuditTrail, createAuditEvent(inquiry.ID, action.ID, "action.executed", DecisionAllow, "executor"))
	return result, nil
}

func (s *WorkflowService) ProcessInquiryWithDuplicateCheck(ctx context.Context, inquiry Inquiry, seen map[string]struct{}) (WorkflowResult, error) {
	key := inquiry.Source + ":" + inquiry.ExternalMessageID
	if _, ok := seen[key]; ok {
		return WorkflowResult{
			ID:         inquiry.ID,
			Inquiry:    inquiry,
			Duplicate:  true,
			AuditTrail: []AuditEvent{createAuditEvent(inquiry.ID, "", "inquiry.duplicate", "DUPLICATE", "idempotency")},
		}, nil
	}
	seen[key] = struct{}{}
	return s.ProcessInquiry(ctx, inquiry)
}

func (s *WorkflowService) ApproveAction(ctx context.Context, actionID, approverID string) (ActionRecord, error) {
	return s.actionService.ApproveAction(ctx, actionID, approverID)
}

func (s *WorkflowService) RejectAction(ctx context.Context, actionID, approverID, reason string) (ActionRecord, error) {
	return s.actionService.RejectAction(ctx, actionID, approverID, reason)
}

func (s *WorkflowService) ExecuteAction(ctx context.Context, actionID, idempotencyKey string) (ActionRecord, error) {
	return s.actionService.ExecuteAction(ctx, actionID, idempotencyKey)
}

func (s *WorkflowService) DenyAction(ctx context.Context, actionID, reason string) (ActionRecord, error) {
	return s.actionService.DenyAction(ctx, actionID, reason)
}

var _ = time.Now
