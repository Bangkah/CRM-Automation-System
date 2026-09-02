package main

import (
	"beda/internal/api"
	"beda/internal/workflow"
	"log"
	"net/http"
)

func main() {
	service := workflow.NewWorkflowService(workflow.MockLLMProvider{}, workflow.MockCRMProvider{})
	handler := api.NewServer(service)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/inquiries", handler)
	mux.Handle("/api/v1/actions/", handler)

	log.Println("BEDA workflow API listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
