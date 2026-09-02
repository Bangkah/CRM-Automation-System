package api

import (
	"beda/internal/workflow"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

const routeInquiries = "/api/v1/inquiries"

type Server struct {
	service *workflow.WorkflowService
}

func NewServer(service *workflow.WorkflowService) *Server {
	if service == nil {
		service = workflow.NewWorkflowService(workflow.MockLLMProvider{}, workflow.MockCRMProvider{})
	}
	return &Server{service: service}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.URL.Path == routeInquiries {
		s.handleCreateInquiry(w, r)
		return
	}
	if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/actions/") {
		s.handleActionDecision(w, r)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "route not found")
}

func (s *Server) handleCreateInquiry(w http.ResponseWriter, r *http.Request) {
	var req inquiryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed_json", "request body must be valid JSON")
		return
	}
	if err := validateRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	inquiry := workflow.Inquiry{
		ID:                fmt.Sprintf("inq-%d", time.Now().UnixNano()),
		Source:            strings.TrimSpace(req.Source),
		ExternalMessageID: strings.TrimSpace(req.ExternalMessageID),
		SenderEmail:       strings.TrimSpace(req.Sender.Email),
		Subject:           strings.TrimSpace(req.Subject),
		Content:           strings.TrimSpace(req.Content),
		ReceivedAt:        time.Now(),
	}

	result, err := s.service.ProcessInquiry(r.Context(), inquiry)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workflow_error", "inquiry could not be processed")
		return
	}

	writeJSON(w, http.StatusOK, inquiryResponse{
		ID:                result.ID,
		ActionID:          result.ActionID,
		Source:            result.Inquiry.Source,
		ExternalMessageID: result.Inquiry.ExternalMessageID,
		Duplicate:         result.Duplicate,
		Classification:    result.Classification,
		Extraction:        result.Extraction,
		PolicyDecision:    result.PolicyDecision,
		Action:            result.ProposedAction,
		AuditTrail:        result.AuditTrail,
	})
}

func (s *Server) handleActionDecision(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/actions/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
		return
	}
	actionID := parts[0]
	actionVerb := parts[1]
	if actionID == "" || (actionVerb != "approve" && actionVerb != "reject") {
		writeError(w, http.StatusBadRequest, "invalid_action_route", "route must be /api/v1/actions/{id}/approve or /reject")
		return
	}

	var req actionDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed_json", "request body must be valid JSON")
		return
	}

	if actionVerb == "approve" {
		updated, err := s.service.ApproveAction(r.Context(), actionID, req.ApproverID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "approval_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, actionDecisionResponse{
			ActionID: updated.ID,
			State:    string(updated.State),
			Decision: workflow.DecisionAllow,
		})
		return
	}

	updated, err := s.service.RejectAction(r.Context(), actionID, req.ApproverID, req.Reason)
	if err != nil {
		writeError(w, http.StatusBadRequest, "rejection_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, actionDecisionResponse{
		ActionID: updated.ID,
		State:    string(updated.State),
		Decision: workflow.DecisionDeny,
	})
}

type inquiryRequest struct {
	Source            string `json:"source"`
	ExternalMessageID string `json:"external_message_id"`
	Sender            sender `json:"sender"`
	Subject           string `json:"subject"`
	Content           string `json:"content"`
}

type sender struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type inquiryResponse struct {
	ID                string                  `json:"id"`
	ActionID          string                  `json:"action_id"`
	Source            string                  `json:"source"`
	ExternalMessageID string                  `json:"external_message_id"`
	Duplicate         bool                    `json:"duplicate"`
	Classification    workflow.Classification `json:"classification"`
	Extraction        workflow.Extraction     `json:"extraction"`
	PolicyDecision    workflow.PolicyDecision `json:"policy_decision"`
	Action            workflow.ActionProposal `json:"action"`
	AuditTrail        []workflow.AuditEvent   `json:"audit_trail"`
}

type actionDecisionRequest struct {
	ApproverID string `json:"approver_id"`
	Reason     string `json:"reason"`
}

type actionDecisionResponse struct {
	ActionID string `json:"action_id"`
	State    string `json:"state"`
	Decision string `json:"decision"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func validateRequest(req inquiryRequest) error {
	if strings.TrimSpace(req.Source) == "" {
		return errors.New("source is required")
	}
	if strings.TrimSpace(req.ExternalMessageID) == "" {
		return errors.New("external_message_id is required")
	}
	if strings.TrimSpace(req.Content) == "" {
		return errors.New("content is required")
	}
	if strings.TrimSpace(req.Sender.Email) != "" {
		if _, err := mail.ParseAddress(req.Sender.Email); err != nil {
			return errors.New("sender.email is invalid")
		}
	}
	if req.Sender.Name != "" && strings.TrimSpace(req.Sender.Name) == "" {
		return errors.New("sender.name is invalid")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: code, Message: message})
}
