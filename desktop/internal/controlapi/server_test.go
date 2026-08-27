package controlapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usbridge/usbridge/desktop/internal/androidclient"
)

func TestStatus(t *testing.T) {
	api := New(Dependencies{Status: func() any {
		return map[string]any{"proxyRunning": true, "controlRunning": true}
	}})
	request := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	response := httptest.NewRecorder()

	api.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("local API must not enable CORS")
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["proxyRunning"] != true {
		t.Fatalf("body = %#v", body)
	}
}

func TestReconnectRequiresJSONAndCallsBackend(t *testing.T) {
	called := false
	api := New(Dependencies{ReconnectMobile: func(context.Context) (androidclient.OperationResponse, error) {
		called = true
		return androidclient.OperationResponse{OK: true, Message: "IP changed"}, nil
	}})

	nonJSON := httptest.NewRequest(http.MethodPost, "/v1/mobile/reconnect", strings.NewReader("{}"))
	nonJSONResponse := httptest.NewRecorder()
	api.ServeHTTP(nonJSONResponse, nonJSON)
	if nonJSONResponse.Code != http.StatusUnsupportedMediaType || called {
		t.Fatalf("status = %d, called = %t", nonJSONResponse.Code, called)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/mobile/reconnect", nil)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !called {
		t.Fatalf("status = %d, called = %t, body = %s", response.Code, called, response.Body.String())
	}
}

func TestSetIPModeRejectsInvalidMode(t *testing.T) {
	called := false
	api := New(Dependencies{SetIPMode: func(context.Context, string) (androidclient.OperationResponse, error) {
		called = true
		return androidclient.OperationResponse{OK: true}, nil
	}})
	request := httptest.NewRequest(http.MethodPut, "/v1/ip-mode", strings.NewReader(`{"mode":"prefer-ipv4"}`))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	response := httptest.NewRecorder()

	api.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || called {
		t.Fatalf("status = %d, called = %t, body = %s", response.Code, called, response.Body.String())
	}
}

func TestWrongMethodIsRejected(t *testing.T) {
	api := New(Dependencies{})
	request := httptest.NewRequest(http.MethodGet, "/v1/mobile/reconnect", nil)
	response := httptest.NewRecorder()

	api.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestHelpDocumentsEveryEndpoint(t *testing.T) {
	api := New(Dependencies{})
	request := httptest.NewRequest(http.MethodGet, "/v1/help", nil)
	response := httptest.NewRecorder()

	api.ServeHTTP(response, request)

	var body helpResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(body.Endpoints) != 9 {
		t.Fatalf("status = %d, endpoints = %d", response.Code, len(body.Endpoints))
	}
}
