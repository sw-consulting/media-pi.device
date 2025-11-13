package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleSystemReboot_CallsRebootAction(t *testing.T) {
	// Use a channel so the RebootAction can notify when it runs.
	done := make(chan struct{})
	old := RebootAction
	RebootAction = func() error { close(done); return nil }
	defer func() { RebootAction = old }()

	req := httptest.NewRequest(http.MethodPost, "/api/menu/system/reboot", nil)
	rr := httptest.NewRecorder()
	HandleSystemReboot(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Result().StatusCode)
	}

	select {
	case <-done:
		// success
	case <-time.After(200 * time.Millisecond):
		var resp APIResponse
		_ = json.NewDecoder(rr.Body).Decode(&resp)
		t.Fatalf("expected RebootAction to be called")
	}
}

func TestHandleSystemShutdown_CallsPowerOffAction(t *testing.T) {
	done2 := make(chan struct{})
	old2 := PowerOffAction
	PowerOffAction = func() error { close(done2); return nil }
	defer func() { PowerOffAction = old2 }()

	req2 := httptest.NewRequest(http.MethodPost, "/api/menu/system/shutdown", nil)
	rr2 := httptest.NewRecorder()
	HandleSystemShutdown(rr2, req2)

	if rr2.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr2.Result().StatusCode)
	}

	select {
	case <-done2:
		// success
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected PowerOffAction to be called")
	}
}
