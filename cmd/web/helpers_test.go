package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_redirectAfterPOST_StackNavRequest_Returns200WithHeaders(t *testing.T) {
	app := &application{
		logger:          nil,
		webAuthnHandler: nil,
		sessionManager:  nil,
		templateFS:      nil,
		workoutService:  nil,
		flightRecorder:  nil,
	}
	tests := []struct {
		name          string
		target        string
		action        string
		wantAction    string
		wantActionSet bool
	}{
		{
			name:          "default replace",
			target:        "/foo",
			action:        "",
			wantAction:    "",
			wantActionSet: false,
		},
		{
			name:          "explicit pop-or-replace",
			target:        "/",
			action:        "pop-or-replace",
			wantAction:    "pop-or-replace",
			wantActionSet: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/whatever", nil)
			r.Header.Set("X-Requested-With", "stacknav")

			app.redirectAfterPOST(w, r, tt.target, tt.action)

			if got := w.Code; got != http.StatusOK {
				t.Errorf("status = %d, want %d", got, http.StatusOK)
			}
			if got := w.Header().Get("X-Location"); got != tt.target {
				t.Errorf("X-Location = %q, want %q", got, tt.target)
			}
			gotAction := w.Header().Get("X-History-Action")
			if tt.wantActionSet {
				if gotAction != tt.wantAction {
					t.Errorf("X-History-Action = %q, want %q", gotAction, tt.wantAction)
				}
			} else if gotAction != "" {
				t.Errorf("X-History-Action = %q, want empty", gotAction)
			}
			if got := w.Body.Len(); got != 0 {
				t.Errorf("body length = %d, want 0", got)
			}
		})
	}
}

func Test_redirectAfterPOST_PlainRequest_Returns303(t *testing.T) {
	app := &application{
		logger:          nil,
		webAuthnHandler: nil,
		sessionManager:  nil,
		templateFS:      nil,
		workoutService:  nil,
		flightRecorder:  nil,
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/whatever", nil)
	// no X-Requested-With

	app.redirectAfterPOST(w, r, "/target", "")

	if got := w.Code; got != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", got, http.StatusSeeOther)
	}
	if got := w.Header().Get("Location"); got != "/target" {
		t.Errorf("Location = %q, want /target", got)
	}
	if got := w.Header().Get("X-Location"); got != "" {
		t.Errorf("X-Location should not be set for plain request, got %q", got)
	}
}
