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
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is supported")
		return
	}
	if r.URL.Path != routeInquiries {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
		return
	}

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

type inquiryRequest struct {
	Source           string `json:"source"`
	ExternalMessageID string `json:"external_message_id"`
	Sender           sender `json:"sender"`
	Subject          string `json:"subject"`
	Content          string `json:"content"`
}

type sender struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type inquiryResponse struct {
	ID                string            `json:"id"`
	Source            string            `json:"source"`
	ExternalMessageID string            `json:"external_message_id"`
	Duplicate         bool              `json:"duplicate"`
	Classification    workflow.Classification `json:"classification"`
	Extraction        workflow.Extraction     `json:"extraction"`
	PolicyDecision    workflow.PolicyDecision `json:"policy_decision"`
	Action            workflow.ActionProposal `json:"action"`
	AuditTrail        []workflow.AuditEvent  `json:"audit_trail"`
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
