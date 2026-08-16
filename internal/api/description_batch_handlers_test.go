package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/pborges/aiinventory/internal/gemini"
)

func TestDescriptionBatchRunsServerSideAndReportsProgress(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	fake := &gemini.Fake{
		DescriptionFunc: func(_ string, _ []string, _ string) (gemini.DescriptionResult, error) {
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
			return gemini.DescriptionResult{Description: "generated description"}, nil
		},
	}
	h, cookies, s := newTestServerWithGemini(t, fake)
	item, err := s.CreateItem(t.Context(), "ZKEI")
	if err != nil {
		t.Fatal(err)
	}

	w := doJSON(t, h, http.MethodPost, "/api/descriptions/batch", map[string]any{
		"items": []map[string]any{{"item_id": item.ID, "hint": "blue"}},
	}, cookies)
	if w.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, body = %s", w.Code, w.Body.String())
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("description worker did not start")
	}

	w = doJSON(t, h, http.MethodGet, "/api/descriptions/batch/status", nil, cookies)
	var status descriptionBatchStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if !status.Exists || !status.Running || len(status.Items) != 1 || status.Items[0].Status != "generating" {
		t.Fatalf("running status = %+v", status)
	}
	if status.Items[0].AssetTag != "ZKEI" {
		t.Fatalf("status asset tag = %q, want ZKEI", status.Items[0].AssetTag)
	}

	createUser := doJSON(t, h, http.MethodPost, "/api/users", credentials{Username: "bob", Password: "correcthorse"}, cookies)
	if createUser.Code != http.StatusCreated {
		t.Fatalf("create second user status = %d, body = %s", createUser.Code, createUser.Body.String())
	}
	login := doJSON(t, h, http.MethodPost, "/api/auth/login", credentials{Username: "bob", Password: "correcthorse"}, nil)
	bobCookies := login.Result().Cookies()
	w = doJSON(t, h, http.MethodGet, "/api/descriptions/batch/status", nil, bobCookies)
	var bobStatus descriptionBatchStatus
	if err := json.NewDecoder(w.Body).Decode(&bobStatus); err != nil {
		t.Fatal(err)
	}
	if bobStatus.Exists || bobStatus.Running || len(bobStatus.Items) != 0 {
		t.Fatalf("second user could observe first user's batch: %+v", bobStatus)
	}

	w = doJSON(t, h, http.MethodPost, "/api/descriptions/batch", map[string]any{
		"items": []map[string]any{{"item_id": item.ID}},
	}, cookies)
	if w.Code != http.StatusConflict {
		t.Fatalf("concurrent start status = %d, want 409", w.Code)
	}
	close(release)
	released = true

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w = doJSON(t, h, http.MethodGet, "/api/descriptions/batch/status", nil, cookies)
		if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
			t.Fatal(err)
		}
		if !status.Running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status.Running || status.Items[0].Status != "done" || status.Items[0].Description != "generated description" {
		t.Fatalf("completed status = %+v", status)
	}

	// The rejected concurrent start above must not consume rate-limit budget:
	// the original one-item job plus this 59-item job exactly fills the limit.
	inputs := make([]map[string]any, 0, 59)
	inputs = append(inputs, map[string]any{"item_id": item.ID})
	for i := 0; i < 58; i++ {
		tag := string([]byte{'A' + byte(i/26), 'A' + byte(i%26), 'X', 'Y'})
		created, err := s.CreateItem(t.Context(), tag)
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, map[string]any{"item_id": created.ID})
	}
	w = doJSON(t, h, http.MethodPost, "/api/descriptions/batch", map[string]any{"items": inputs}, cookies)
	if w.Code != http.StatusAccepted {
		t.Fatalf("rate budget was charged for rejected start: status = %d, body = %s", w.Code, w.Body.String())
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		w = doJSON(t, h, http.MethodGet, "/api/descriptions/batch/status", nil, cookies)
		if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
			t.Fatal(err)
		}
		if !status.Running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status.Running {
		t.Fatal("second description batch did not finish")
	}
}
