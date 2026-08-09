package updater

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPHandlerExposesOnlyNarrowOperationAPI(t *testing.T) {
	controller, _ := newTestController(t)
	handler, err := NewHTTPHandler(NewService(controller))
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/operations", bytes.NewBufferString(`{"release_id":"ov-dash-2.0.0","command":"sh"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/operations", bytes.NewBufferString(`{"release_id":"ov-dash-2.0.0"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/commands", bytes.NewBufferString(`{"name":"docker"}`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("command endpoint status = %d", response.Code)
	}

	active, err := controller.Active()
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		_, _ = controller.Wait(context.Background(), active.ID)
	}
}

func TestCommandValidationRejectsShellsAndScriptFiles(t *testing.T) {
	for _, name := range []string{"sh", "/bin/bash", "powershell.exe", "update.cmd", "update.ps1"} {
		if err := validateCommand(Command{Name: name}); err == nil {
			t.Fatalf("validateCommand(%q) succeeded", name)
		}
	}
	if err := validateCommand(Command{Name: "docker", Args: []string{"compose", "up"}}); err != nil {
		t.Fatalf("docker command rejected: %v", err)
	}
}
