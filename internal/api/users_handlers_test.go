package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
)

func TestUserManagementFlow(t *testing.T) {
	h, cookies, _ := newTestServerWithGemini(t, nil)

	// only alice (the bootstrap account) exists so far
	listResp := doJSON(t, h, http.MethodGet, "/api/users", nil, cookies)
	var listBody struct {
		Users []userListItemResponse `json:"users"`
	}
	json.NewDecoder(listResp.Body).Decode(&listBody)
	if len(listBody.Users) != 1 || listBody.Users[0].Username != "alice" || !listBody.Users[0].Enabled {
		t.Fatalf("users = %+v, want just alice, enabled", listBody.Users)
	}

	// create a second account
	createResp := doJSON(t, h, http.MethodPost, "/api/users", credentials{Username: "bob", Password: "correcthorse"}, cookies)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createResp.Code, createResp.Body.String())
	}
	var createBody struct {
		User userResponse `json:"user"`
	}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	if createBody.User.Username != "bob" || !createBody.User.Enabled {
		t.Fatalf("created user = %+v", createBody.User)
	}

	// duplicate username is rejected
	dupResp := doJSON(t, h, http.MethodPost, "/api/users", credentials{Username: "bob", Password: "correcthorse"}, cookies)
	if dupResp.Code != http.StatusConflict {
		t.Fatalf("duplicate create status = %d, want 409", dupResp.Code)
	}

	// bob can log in
	loginResp := doJSON(t, h, http.MethodPost, "/api/auth/login", credentials{Username: "bob", Password: "correcthorse"}, nil)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("bob login status = %d, body = %s", loginResp.Code, loginResp.Body.String())
	}
	bobCookies := loginResp.Result().Cookies()

	// disable bob (as alice, since there's no admin distinction — any user can)
	disableResp := doJSON(t, h, http.MethodPut, "/api/users/"+strconv.FormatInt(createBody.User.ID, 10), setUserEnabledRequest{Enabled: false}, cookies)
	if disableResp.Code != http.StatusNoContent {
		t.Fatalf("disable status = %d, body = %s", disableResp.Code, disableResp.Body.String())
	}

	// bob's existing session is now rejected immediately
	meResp := doJSON(t, h, http.MethodGet, "/api/auth/me", nil, bobCookies)
	if meResp.Code != http.StatusUnauthorized {
		t.Fatalf("disabled bob's /me status = %d, want 401", meResp.Code)
	}

	// and bob can no longer log in fresh either
	reloginResp := doJSON(t, h, http.MethodPost, "/api/auth/login", credentials{Username: "bob", Password: "correcthorse"}, nil)
	if reloginResp.Code != http.StatusUnauthorized {
		t.Fatalf("disabled bob relogin status = %d, want 401", reloginResp.Code)
	}

	// re-enable
	enableResp := doJSON(t, h, http.MethodPut, "/api/users/"+strconv.FormatInt(createBody.User.ID, 10), setUserEnabledRequest{Enabled: true}, cookies)
	if enableResp.Code != http.StatusNoContent {
		t.Fatalf("re-enable status = %d", enableResp.Code)
	}
	reloginResp2 := doJSON(t, h, http.MethodPost, "/api/auth/login", credentials{Username: "bob", Password: "correcthorse"}, nil)
	if reloginResp2.Code != http.StatusOK {
		t.Fatalf("re-enabled bob relogin status = %d", reloginResp2.Code)
	}
}

func TestUserManagementRequiresAuth(t *testing.T) {
	h, _, _ := newTestServerWithGemini(t, nil)
	w := doJSON(t, h, http.MethodGet, "/api/users", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
