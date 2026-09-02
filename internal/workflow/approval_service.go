package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ActionService bundles proposal, approval, denial, and execution logic.
type ActionService struct {
	repo  ActionRepository
	audit AuditRepository
}

func NewActionService(repo ActionRepository, audit AuditRepository) *ActionService {
	if repo == nil {
		repo = newInMemoryActionRepository()
	}
	if audit == nil {
		audit = newInMemoryAuditRepository()
	}
	return &ActionService{repo: repo, audit: audit}
}

func (s *ActionService) CreateProposal(ctx context.Context, inquiryID, actionType, description, policyDecision string, requiresApproval, highRisk bool) (ActionRecord, error) {
	if inquiryID == "" {
		return ActionRecord{}, errors.New("inquiry id is required")
	}
	if actionType == "" {
		return ActionRecord{}, errors.New("action type is required")
	}
	action := ActionRecord{
		ID:               fmt.Sprintf("act-%d", time.Now().UnixNano()),
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
	if err := s.repo.Create(ctx, action); err != nil {
		return ActionRecord{}, err
	}
	if err := s.audit.Append(ctx, createAuditEvent(inquiryID, action.ID, "action.proposed", policyDecision, "workflow")); err != nil {
		return ActionRecord{}, err
	}
	return action, nil
}

func (s *ActionService) RequestApproval(ctx context.Context, actionID, approverID string) (ActionRecord, error) {
	if approverID == "" {
		approverID = "human-reviewer"
	}
	action, err := s.repo.Get(ctx, actionID)
	if err != nil {
		return ActionRecord{}, err
	}
	if action.State != ActionStateProposed {
		return ActionRecord{}, fmt.Errorf("action %s is not in PROPOSED state", actionID)
	}
	updated := action
	updated.State = ActionStatePendingApproval
	updated.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, updated); err != nil {
		return ActionRecord{}, err
	}
	if err := s.audit.Append(ctx, createAuditEvent(action.InquiryID, action.ID, "approval.requested", DecisionRequireApproval, "approval")); err != nil {
		return ActionRecord{}, err
	}
	return updated, nil
}

func (s *ActionService) ApproveAction(ctx context.Context, actionID, approverID string) (ActionRecord, error) {
	if approverID == "" {
		approverID = "human-reviewer"
	}
	action, err := s.repo.Get(ctx, actionID)
	if err != nil {
		return ActionRecord{}, err
	}
	if action.State != ActionStatePendingApproval {
		return ActionRecord{}, fmt.Errorf("invalid approval transition from %s", action.State)
	}
	updated := action
	updated.State = ActionStateApproved
	updated.ApprovedBy = approverID
	updated.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, updated); err != nil {
		return ActionRecord{}, err
	}
	if err := s.audit.Append(ctx, createAuditEvent(action.InquiryID, action.ID, "approval.granted", DecisionAllow, "approval")); err != nil {
		return ActionRecord{}, err
	}
	return updated, nil
}

func (s *ActionService) RejectAction(ctx context.Context, actionID, approverID, reason string) (ActionRecord, error) {
	if approverID == "" {
		approverID = "human-reviewer"
	}
	action, err := s.repo.Get(ctx, actionID)
	if err != nil {
		return ActionRecord{}, err
	}
	if action.State != ActionStatePendingApproval {
		return ActionRecord{}, fmt.Errorf("invalid rejection transition from %s", action.State)
	}
	updated := action
	updated.State = ActionStateRejected
	updated.RejectedBy = approverID
	updated.LastError = reason
	updated.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, updated); err != nil {
		return ActionRecord{}, err
	}
	if err := s.audit.Append(ctx, createAuditEvent(action.InquiryID, action.ID, "approval.rejected", DecisionDeny, "approval")); err != nil {
		return ActionRecord{}, err
	}
	return updated, nil
}

func (s *ActionService) DenyAction(ctx context.Context, actionID, reason string) (ActionRecord, error) {
	if actionID == "" {
		return ActionRecord{}, errors.New("action id is required")
	}
	action, err := s.repo.Get(ctx, actionID)
	if err != nil {
		return ActionRecord{}, err
	}
	if action.State == ActionStateExecuted || action.State == ActionStateRejected || action.State == ActionStateDenied || action.State == ActionStateFailed {
		return ActionRecord{}, fmt.Errorf("invalid denial transition from %s", action.State)
	}
	updated := action
	updated.State = ActionStateDenied
	updated.LastError = reason
	updated.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, updated); err != nil {
		return ActionRecord{}, err
	}
	if err := s.audit.Append(ctx, createAuditEvent(action.InquiryID, action.ID, "action.denied", DecisionDeny, "policy")); err != nil {
		return ActionRecord{}, err
	}
	return updated, nil
}

func (s *ActionService) ExecuteAction(ctx context.Context, actionID, executionID string) (ActionRecord, error) {
	if actionID == "" {
		return ActionRecord{}, errors.New("action id is required")
	}
	action, err := s.repo.Get(ctx, actionID)
	if err != nil {
		return ActionRecord{}, err
	}
	if action.State == ActionStateExecuted {
		return action, nil
	}
	if action.State == ActionStateRejected || action.State == ActionStateDenied || action.State == ActionStateFailed {
		return ActionRecord{}, fmt.Errorf("invalid execution transition from %s", action.State)
	}
	if action.PolicyDecision == DecisionDeny {
		return ActionRecord{}, errors.New("denied actions can never execute")
	}
	if action.PolicyDecision == DecisionRequireApproval && action.State != ActionStateApproved {
		return ActionRecord{}, errors.New("requires approval before execution")
	}
	if action.PolicyDecision == DecisionAllow && action.State == ActionStatePendingApproval {
		return ActionRecord{}, errors.New("requires approval before execution")
	}

	updated := action
	updated.State = ActionStateExecuted
	updated.IdempotencyKey = executionID
	updated.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, updated); err != nil {
		updated.State = ActionStateFailed
		updated.LastError = err.Error()
		_ = s.repo.Update(ctx, updated)
		_ = s.audit.Append(ctx, createAuditEvent(action.InquiryID, action.ID, "action.failed", DecisionDeny, "executor"))
		return ActionRecord{}, err
	}
	if err := s.audit.Append(ctx, createAuditEvent(action.InquiryID, action.ID, "action.executed", DecisionAllow, "executor")); err != nil {
		return ActionRecord{}, err
	}
	return updated, nil
}
