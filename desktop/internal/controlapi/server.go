package controlapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/usbridge/usbridge/desktop/internal/androidclient"
	"github.com/usbridge/usbridge/desktop/internal/traffic"
)

const maxRequestBodyBytes = 64 << 10

type Operation func(context.Context) (androidclient.OperationResponse, error)

type Dependencies struct {
	Status                func() any
	Traffic               func() traffic.Snapshot
	ReconnectMobile       Operation
	RefreshPublicIP       Operation
	ForceCellularUpstream Operation
	StartTethering        Operation
	StopTethering         Operation
	SetIPMode             func(context.Context, string) (androidclient.OperationResponse, error)
}

type API struct {
	dependencies Dependencies
	handler      http.Handler
}

type errorResponse struct {
	OK    bool     `json:"ok"`
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ipModeRequest struct {
	Mode string `json:"mode"`
}

type endpointHelp struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Description string `json:"description"`
	Body        any    `json:"body,omitempty"`
}

type helpResponse struct {
	Name           string         `json:"name"`
	Version        string         `json:"version"`
	BaseURL        string         `json:"baseUrl"`
	Authentication string         `json:"authentication"`
	Endpoints      []endpointHelp `json:"endpoints"`
}

func New(dependencies Dependencies) *API {
	api := &API{dependencies: dependencies}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", api.status)
	mux.HandleFunc("GET /v1/traffic", api.traffic)
	mux.HandleFunc("GET /v1/help", api.help)
	mux.HandleFunc("POST /v1/mobile/reconnect", api.operation(dependencies.ReconnectMobile))
	mux.HandleFunc("POST /v1/public-ip/refresh", api.operation(dependencies.RefreshPublicIP))
	mux.HandleFunc("POST /v1/upstream/cellular", api.operation(dependencies.ForceCellularUpstream))
	mux.HandleFunc("POST /v1/tether/start", api.operation(dependencies.StartTethering))
	mux.HandleFunc("POST /v1/tether/stop", api.operation(dependencies.StopTethering))
	mux.HandleFunc("PUT /v1/ip-mode", api.setIPMode)
	api.handler = api.responseHeaders(mux)
	return api
}

func (api *API) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	api.handler.ServeHTTP(writer, request)
}

func NewHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      4 * time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
}

func (api *API) responseHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(writer, request)
	})
}

func (api *API) status(writer http.ResponseWriter, _ *http.Request) {
	if api.dependencies.Status == nil {
		writeError(writer, http.StatusServiceUnavailable, "service_unavailable", "Windows service is still starting")
		return
	}
	writeJSON(writer, http.StatusOK, api.dependencies.Status())
}

func (api *API) traffic(writer http.ResponseWriter, _ *http.Request) {
	if api.dependencies.Traffic == nil {
		writeError(writer, http.StatusServiceUnavailable, "service_unavailable", "traffic meter is unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, api.dependencies.Traffic())
}

func (api *API) help(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, helpResponse{
		Name:           "USBridge Desktop Local Control API",
		Version:        "v1",
		BaseURL:        "http://127.0.0.1:18082",
		Authentication: "none; loopback access only",
		Endpoints: []endpointHelp{
			{Method: http.MethodGet, Path: "/v1/help", Description: "Read this machine-readable endpoint list"},
			{Method: http.MethodGet, Path: "/v1/status", Description: "Read Windows, proxy, USB adapter and phone status"},
			{Method: http.MethodPost, Path: "/v1/mobile/reconnect", Description: "Reconnect mobile data and verify whether the public IP changed", Body: map[string]any{}},
			{Method: http.MethodPost, Path: "/v1/public-ip/refresh", Description: "Refresh the phone cellular public IP values", Body: map[string]any{}},
			{Method: http.MethodPost, Path: "/v1/upstream/cellular", Description: "Force USB tethering to use the cellular upstream", Body: map[string]any{}},
			{Method: http.MethodPost, Path: "/v1/tether/start", Description: "Start USB tethering on the phone", Body: map[string]any{}},
			{Method: http.MethodPost, Path: "/v1/tether/stop", Description: "Stop USB tethering on the phone", Body: map[string]any{}},
			{Method: http.MethodPut, Path: "/v1/ip-mode", Description: "Set auto, IPv4 or IPv6 mode", Body: ipModeRequest{Mode: "auto"}},
			{Method: http.MethodGet, Path: "/v1/traffic", Description: "Read traffic measured by the Windows proxy"},
		},
	})
}

func (api *API) operation(operation Operation) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if !requireJSON(writer, request) {
			return
		}
		if err := decodeOptionalEmptyObject(writer, request); err != nil {
			return
		}
		if operation == nil {
			writeError(writer, http.StatusServiceUnavailable, "service_unavailable", "operation is unavailable")
			return
		}
		result, err := operation(request.Context())
		if err != nil {
			writeError(writer, http.StatusBadGateway, "phone_control_failed", err.Error())
			return
		}
		writeJSON(writer, http.StatusOK, result)
	}
}

func (api *API) setIPMode(writer http.ResponseWriter, request *http.Request) {
	if !requireJSON(writer, request) {
		return
	}
	var input ipModeRequest
	if err := decodeJSON(writer, request, &input, false); err != nil {
		return
	}
	input.Mode = strings.ToLower(strings.TrimSpace(input.Mode))
	if input.Mode != "auto" && input.Mode != "ipv4" && input.Mode != "ipv6" {
		writeError(writer, http.StatusBadRequest, "invalid_ip_mode", "mode must be auto, ipv4 or ipv6")
		return
	}
	if api.dependencies.SetIPMode == nil {
		writeError(writer, http.StatusServiceUnavailable, "service_unavailable", "IP mode control is unavailable")
		return
	}
	result, err := api.dependencies.SetIPMode(request.Context(), input.Mode)
	if err != nil {
		writeError(writer, http.StatusBadGateway, "phone_control_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func requireJSON(writer http.ResponseWriter, request *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		writeError(writer, http.StatusUnsupportedMediaType, "json_required", "Content-Type must be application/json")
		return false
	}
	return true
}

func decodeOptionalEmptyObject(writer http.ResponseWriter, request *http.Request) error {
	var input struct{}
	return decodeJSON(writer, request, &input, true)
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, target any, allowEmpty bool) error {
	request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if allowEmpty && err == io.EOF {
			return nil
		}
		writeError(writer, http.StatusBadRequest, "invalid_json", fmt.Sprintf("invalid JSON body: %v", err))
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values are not allowed")
		}
		writeError(writer, http.StatusBadRequest, "invalid_json", fmt.Sprintf("invalid JSON body: %v", err))
		return err
	}
	return nil
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, errorResponse{OK: false, Error: apiError{Code: code, Message: message}})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
