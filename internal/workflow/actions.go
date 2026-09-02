package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type ActionState string

const (
	ActionStateProposed        ActionState = "PROPOSED"
	ActionStatePendingApproval ActionState = "PENDING_APPROVAL"
	ActionStateApproved        ActionState = "APPROVED"
	ActionStateExecuted        ActionState = "EXECUTED"
	ActionStateRejected        ActionState = "REJECTED"
	ActionStateDenied          ActionState = "DENIED"
	ActionStateFailed          ActionState = "FAILED"
)

func validActionStateTransition(from, to ActionState) bool {
	if from == ActionStateProposed && to == ActionStatePendingApproval {
		return true
	}
	if from == ActionStateProposed && to == ActionStateApproved {
		return true
	}
	if from == ActionStateProposed && to == ActionStateRejected {
		return true
	}
	if from == ActionStateProposed && to == ActionStateDenied {
		return true
	}
	if from == ActionStatePendingApproval && to == ActionStateApproved {
		return true
	}
	if from == ActionStatePendingApproval && to == ActionStateRejected {
		return true
	}
	if from == ActionStateApproved && to == ActionStateExecuted {
		return true
	}
	if from == ActionStateApproved && to == ActionStateFailed {
		return true
	}
	if from == ActionStateProposed && to == ActionStateFailed {
		return true
	}
	if from == ActionStatePendingApproval && to == ActionStateFailed {
		return true
	}
	return false
}

// ActionRecord stores the explicit lifecycle of an action proposal.
type ActionRecord struct {
	ID               string
	InquiryID        string
	Type             string
	Description      string
	PolicyDecision   string
	State            ActionState
	RequiresApproval bool
	HighRisk         bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
	ApprovedBy       string
	RejectedBy       string
	IdempotencyKey   string
	LastError        string
}

// ActionRepository provides persistent storage for action records.
type ActionRepository interface {
	Create(ctx context.Context, action ActionRecord) error
	Get(ctx context.Context, id string) (ActionRecord, error)
	Update(ctx context.Context, action ActionRecord) error
}

// AuditRepository stores append-only workflow events.
type AuditRepository interface {
	Append(ctx context.Context, event AuditEvent) error
	ListForAction(ctx context.Context, actionID string) ([]AuditEvent, error)
	ListForInquiry(ctx context.Context, inquiryID string) ([]AuditEvent, error)
}

// ActionProposer proposes deterministic actions from workflow results.
type ActionProposer struct {
	repo  ActionRepository
	audit AuditRepository
}

func (p ActionProposer) ProposeAction(ctx context.Context, inquiryID, actionType, description, policyDecision string, requiresApproval, highRisk bool) (ActionRecord, error) {
	if inquiryID == "" {
		return ActionRecord{}, errors.New("inquiry id is required")
	}
	id := fmt.Sprintf("act-%d", time.Now().UnixNano())
	action := ActionRecord{
		ID:               id,
		InquiryID:        inquiryID,
		Type:             actionType,
		Description:      description,
		PolicyDecision:   policyDecision,
		State:            ActionStateProposed,
		RequiresApproval: requiresApproval,
		HighRisk:         highRisk,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := p.repo.Create(ctx, action); err != nil {
		return ActionRecord{}, fmt.Errorf("create action: %w", err)
	}
	if err := p.audit.Append(ctx, createAuditEvent(inquiryID, action.ID, "action.proposed", policyDecision, "workflow")); err != nil {
		return ActionRecord{}, fmt.Errorf("append proposal audit: %w", err)
	}
	return action, nil
}

// ApprovalService performs explicit human approval state changes.
type ApprovalService struct {
	repo  ActionRepository
	audit AuditRepository
}

func (s ApprovalService) ApproveAction(ctx context.Context, actionID, approverID string) (ActionRecord, error) {
	if actionID == "" {
		return ActionRecord{}, errors.New("action id is required")
	}
	if approverID == "" {
		approverID = "human-reviewer"
	}
	action, err := s.repo.Get(ctx, actionID)
	if err != nil {
		return ActionRecord{}, fmt.Errorf("get action: %w", err)
	}
	if action.State == ActionStateExecuted || action.State == ActionStateRejected || action.State == ActionStateDenied || action.State == ActionStateFailed {
		return ActionRecord{}, fmt.Errorf("invalid transition from state %s to APPROVED", action.State)
	}
	if action.State != ActionStatePendingApproval && action.State != ActionStateProposed {
		return ActionRecord{}, fmt.Errorf("invalid transition from state %s to APPROVED", action.State)
	}
	updated := action
	updated.State = ActionStateApproved
	updated.ApprovedBy = approverID
	updated.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, updated); err != nil {
		return ActionRecord{}, fmt.Errorf("save approved action: %w", err)
	}
	if err := s.audit.Append(ctx, createAuditEvent(action.InquiryID, action.ID, "approval.granted", DecisionAllow, "approval")); err != nil {
		return ActionRecord{}, fmt.Errorf("append approval audit: %w", err)
	}
	return updated, nil
}

func (s ApprovalService) RejectAction(ctx context.Context, actionID, approverID, reason string) (ActionRecord, error) {
	if actionID == "" {
		return ActionRecord{}, errors.New("action id is required")
	}
	if approverID == "" {
		approverID = "human-reviewer"
	}
	action, err := s.repo.Get(ctx, actionID)
	if err != nil {
		return ActionRecord{}, fmt.Errorf("get action: %w", err)
	}
	if action.State == ActionStateExecuted || action.State == ActionStateRejected || action.State == ActionStateDenied || action.State == ActionStateFailed {
		return ActionRecord{}, fmt.Errorf("invalid transition from state %s to REJECTED", action.State)
	}
	if action.State != ActionStatePendingApproval && action.State != ActionStateProposed {
		return ActionRecord{}, fmt.Errorf("invalid transition from state %s to REJECTED", action.State)
	}
	updated := action
	updated.State = ActionStateRejected
	updated.RejectedBy = approverID
	updated.LastError = reason
	updated.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, updated); err != nil {
		return ActionRecord{}, fmt.Errorf("save rejected action: %w", err)
	}
	if err := s.audit.Append(ctx, createAuditEvent(action.InquiryID, action.ID, "approval.rejected", DecisionDeny, "approval")); err != nil {
		return ActionRecord{}, fmt.Errorf("append rejection audit: %w", err)
	}
	return updated, nil
}

// ActionExecutor executes authorized actions only once.
type ActionExecutor struct {
	repo  ActionRepository
	audit AuditRepository
}

func (e ActionExecutor) ExecuteAction(ctx context.Context, actionID, idempotencyKey string) (ActionRecord, error) {
	if actionID == "" {
		return ActionRecord{}, errors.New("action id is required")
	}
	action, err := e.repo.Get(ctx, actionID)
	if err != nil {
		return ActionRecord{}, fmt.Errorf("get action: %w", err)
	}
	if action.State == ActionStateExecuted {
		return action, nil
	}
	if action.State == ActionStateRejected || action.State == ActionStateDenied || action.State == ActionStateFailed {
		return ActionRecord{}, errors.New("action cannot execute from terminal state")
	}
	if action.PolicyDecision == DecisionDeny {
		return ActionRecord{}, errors.New("deny policy blocks execution")
	}
	if action.PolicyDecision == DecisionRequireApproval && action.State != ActionStateApproved {
		return ActionRecord{}, errors.New("action requires approval before execution")
	}
	if action.PolicyDecision == DecisionAllow && action.State == ActionStatePendingApproval {
		return ActionRecord{}, errors.New("action requires approval before execution")
	}

	updated := action
	updated.IdempotencyKey = idempotencyKey
	updated.State = ActionStateExecuted
	updated.UpdatedAt = time.Now()
	if err := e.repo.Update(ctx, updated); err != nil {
		updated.State = ActionStateFailed
		updated.LastError = err.Error()
		_ = e.repo.Update(ctx, updated)
		_ = e.audit.Append(ctx, createAuditEvent(action.InquiryID, action.ID, "action.failed", DecisionDeny, "executor"))
		return ActionRecord{}, fmt.Errorf("save executed action: %w", err)
	}
	if err := e.audit.Append(ctx, createAuditEvent(action.InquiryID, action.ID, "action.executed", DecisionAllow, "executor")); err != nil {
		return ActionRecord{}, fmt.Errorf("append execution audit: %w", err)
	}
	return updated, nil
}

// ActionPolicy provides deterministic decision mapping and execution gating.
type ActionPolicy struct{}

func (ActionPolicy) Evaluate(classification Classification, extraction Extraction, crm CRMMatch, actionType string) PolicyDecision {
	if classification.Category == CategorySpam {
		return PolicyDecision{Decision: DecisionDeny, Reason: "spam inquiry is not actionable"}
	}
	if stringsContainsAny(actionType, "message", "email", "external") {
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

func stringsContainsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if len(candidate) == 0 {
			continue
		}
		if len(value) >= len(candidate) && value == candidate {
			return true
		}
		if len(value) >= len(candidate) && containsSubstring(value, candidate) {
			return true
		}
	}
	return false
}

func containsSubstring(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})())
}

type inMemoryActionRepository struct {
	mu      sync.RWMutex
	actions map[string]ActionRecord
}

func newInMemoryActionRepository() *inMemoryActionRepository {
	return &inMemoryActionRepository{actions: map[string]ActionRecord{}}
}

func (r *inMemoryActionRepository) Create(_ context.Context, action ActionRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.actions[action.ID]; ok {
		return fmt.Errorf("action %s already exists", action.ID)
	}
	r.actions[action.ID] = action
	return nil
}

func (r *inMemoryActionRepository) Get(_ context.Context, id string) (ActionRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	action, ok := r.actions[id]
	if !ok {
		return ActionRecord{}, fmt.Errorf("action %s not found", id)
	}
	return action, nil
}

func (r *inMemoryActionRepository) Update(_ context.Context, action ActionRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.actions[action.ID]; !ok {
		return fmt.Errorf("action %s not found", action.ID)
	}
	r.actions[action.ID] = action
	return nil
}

type inMemoryAuditRepository struct {
	mu     sync.RWMutex
	events map[string][]AuditEvent
}

func newInMemoryAuditRepository() *inMemoryAuditRepository {
	return &inMemoryAuditRepository{events: map[string][]AuditEvent{}}
}

func (r *inMemoryAuditRepository) Append(_ context.Context, event AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := event.InquiryID
	if event.ActionID != "" {
		key = event.ActionID
	}
	r.events[key] = append(r.events[key], event)
	return nil
}

func (r *inMemoryAuditRepository) ListForAction(_ context.Context, actionID string) ([]AuditEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]AuditEvent(nil), r.events[actionID]...), nil
}

func (r *inMemoryAuditRepository) ListForInquiry(_ context.Context, inquiryID string) ([]AuditEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]AuditEvent(nil), r.events[inquiryID]...), nil
}
