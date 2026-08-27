//go:build windows

package exclusivenet

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

func TestDisableExitsHelperWhenLocalStateIsInactive(t *testing.T) {
	manager, requests, finish := connectedTestController(t, false, Target{})
	defer finish()

	manager.Configure(false)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.Reconcile(ctx, nil); err != nil {
		t.Fatal(err)
	}

	request := <-requests
	if request.Command != helperCommandExit {
		t.Fatalf("disable command = %q, want %q", request.Command, helperCommandExit)
	}
	if manager.connection != nil {
		t.Fatal("helper connection remained open after exclusive mode was disabled")
	}
}

func TestFailedApplyDropsHelperAndOldPolicyState(t *testing.T) {
	manager, requests, finish := connectedTestController(t, false, Target{})
	defer finish()
	manager.Configure(true)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- manager.Reconcile(ctx, &Target{ID: "new", Name: "Phone USB", InterfaceIndex: 42})
	}()

	request := <-requests
	if request.Command != helperCommandApply {
		t.Fatalf("apply command = %q, want %q", request.Command, helperCommandApply)
	}
	requests <- helperRequest{
		ID:             request.ID,
		Command:        "respond-error",
		InterfaceIndex: 17,
	}
	if err := <-done; err == nil {
		t.Fatal("expected apply failure")
	}
	if manager.connection != nil {
		t.Fatal("helper connection remained open after apply failure")
	}
	if manager.Status().Active {
		t.Fatal("exclusive mode remained active after apply failure")
	}
}

func TestCheckDropsHelperWhenPolicyNoLongerMatches(t *testing.T) {
	target := Target{ID: "phone", Name: "Phone USB", InterfaceIndex: 42}
	manager, requests, finish := connectedTestController(t, true, target)
	defer finish()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		manager.Check(ctx)
		close(done)
	}()

	request := <-requests
	if request.Command != helperCommandPing {
		t.Fatalf("check command = %q, want %q", request.Command, helperCommandPing)
	}
	requests <- helperRequest{ID: request.ID, Command: "respond-inactive"}
	<-done

	status := manager.Status()
	if status.Active || status.Error == "" {
		t.Fatalf("unexpected status after failed heartbeat: %+v", status)
	}
	if manager.connection != nil {
		t.Fatal("helper connection remained open after policy mismatch")
	}
}

func TestFaultRequiresDisableBeforeReconcile(t *testing.T) {
	manager := &windowsController{
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		enabled: true,
		faulted: true,
		lastErr: "保护已停止",
	}
	target := &Target{ID: "phone", Name: "Phone USB", InterfaceIndex: 42}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.Reconcile(ctx, target); err == nil || err.Error() != "保护已停止" {
		t.Fatalf("faulted reconcile error = %v", err)
	}
	if manager.connection != nil {
		t.Fatal("faulted reconcile unexpectedly started a helper")
	}

	manager.Configure(false)
	manager.Configure(true)
	if err := manager.Reconcile(ctx, target); err == nil || err.Error() == "保护已停止" {
		t.Fatalf("fault latch was not cleared by disable/enable: %v", err)
	}
}

func connectedTestController(t *testing.T, active bool, target Target) (*windowsController, chan helperRequest, func()) {
	t.Helper()
	client, server := net.Pipe()
	requests := make(chan helperRequest)
	manager := &windowsController{
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		enabled:    true,
		active:     active,
		target:     target,
		connection: client,
		encoder:    json.NewEncoder(client),
		decoder:    json.NewDecoder(client),
	}

	go func() {
		decoder := json.NewDecoder(server)
		encoder := json.NewEncoder(server)
		for {
			var request helperRequest
			if err := decoder.Decode(&request); err != nil {
				return
			}
			requests <- request
			if request.Command == helperCommandExit {
				_ = encoder.Encode(helperResponse{ID: request.ID, OK: true})
				return
			}
			instruction := <-requests
			response := helperResponse{ID: request.ID, OK: true, Active: true, InterfaceIndex: request.InterfaceIndex}
			switch instruction.Command {
			case "respond-error":
				response.OK = false
				response.Active = true
				response.InterfaceIndex = instruction.InterfaceIndex
				response.Error = "apply failed"
			case "respond-inactive":
				response.Active = false
				response.InterfaceIndex = 0
			}
			if err := encoder.Encode(response); err != nil {
				return
			}
		}
	}()

	finish := func() {
		_ = client.Close()
		_ = server.Close()
	}
	return manager, requests, finish
}
