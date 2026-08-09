package store

import (
	"context"
	"testing"
)

func TestUserSettingGetSetDelete(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	alice, err := s.CreateFirstUser(ctx, "alice", "hash")
	if err != nil {
		t.Fatalf("CreateFirstUser: %v", err)
	}

	if _, ok, err := s.GetUserSetting(ctx, alice.ID, "nope"); err != nil || ok {
		t.Fatalf("GetUserSetting on missing key = (ok=%v, err=%v), want (false, nil)", ok, err)
	}

	if err := s.SetUserSetting(ctx, alice.ID, "theme", "dark"); err != nil {
		t.Fatalf("SetUserSetting: %v", err)
	}
	if v, ok, err := s.GetUserSetting(ctx, alice.ID, "theme"); err != nil || !ok || v != "dark" {
		t.Fatalf("GetUserSetting = (%q, %v, %v), want (dark, true, nil)", v, ok, err)
	}

	// overwrite
	if err := s.SetUserSetting(ctx, alice.ID, "theme", "light"); err != nil {
		t.Fatalf("SetUserSetting overwrite: %v", err)
	}
	if v, _, _ := s.GetUserSetting(ctx, alice.ID, "theme"); v != "light" {
		t.Fatalf("GetUserSetting after overwrite = %q, want light", v)
	}

	if err := s.DeleteUserSetting(ctx, alice.ID, "theme"); err != nil {
		t.Fatalf("DeleteUserSetting: %v", err)
	}
	if _, ok, err := s.GetUserSetting(ctx, alice.ID, "theme"); err != nil || ok {
		t.Fatalf("GetUserSetting after delete = (ok=%v, err=%v), want (false, nil)", ok, err)
	}
}

func TestUserSettingsScopedPerUser(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	alice, err := s.CreateFirstUser(ctx, "alice", "hash")
	if err != nil {
		t.Fatalf("CreateFirstUser: %v", err)
	}
	bob, err := s.CreateUser(ctx, "bob", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := s.SetUserSetting(ctx, alice.ID, "theme", "dark"); err != nil {
		t.Fatalf("SetUserSetting alice: %v", err)
	}
	if _, ok, err := s.GetUserSetting(ctx, bob.ID, "theme"); err != nil || ok {
		t.Fatalf("bob's GetUserSetting = (ok=%v, err=%v), want (false, nil) — settings must not leak across users", ok, err)
	}
}

type testUserSettingShape struct {
	Rows int    `json:"rows"`
	Name string `json:"name"`
}

func TestUserSettingJSON(t *testing.T) {
	ctx := context.Background()
	s := NewTestStore(t)
	alice, err := s.CreateFirstUser(ctx, "alice", "hash")
	if err != nil {
		t.Fatalf("CreateFirstUser: %v", err)
	}
	def := testUserSettingShape{Rows: 6, Name: "default"}

	// no override yet: falls back to def
	got, err := GetUserSettingJSON(ctx, s, alice.ID, "shape", def)
	if err != nil {
		t.Fatalf("GetUserSettingJSON: %v", err)
	}
	if got != def {
		t.Fatalf("GetUserSettingJSON with no override = %+v, want default %+v", got, def)
	}

	override := testUserSettingShape{Rows: 9, Name: "custom"}
	if err := SetUserSettingJSON(ctx, s, alice.ID, "shape", override); err != nil {
		t.Fatalf("SetUserSettingJSON: %v", err)
	}
	got, err = GetUserSettingJSON(ctx, s, alice.ID, "shape", def)
	if err != nil {
		t.Fatalf("GetUserSettingJSON after set: %v", err)
	}
	if got != override {
		t.Fatalf("GetUserSettingJSON after set = %+v, want override %+v", got, override)
	}

	// restore defaults: delete the override, falls back to def again
	if err := s.DeleteUserSetting(ctx, alice.ID, "shape"); err != nil {
		t.Fatalf("DeleteUserSetting: %v", err)
	}
	got, err = GetUserSettingJSON(ctx, s, alice.ID, "shape", def)
	if err != nil {
		t.Fatalf("GetUserSettingJSON after delete: %v", err)
	}
	if got != def {
		t.Fatalf("GetUserSettingJSON after delete = %+v, want default %+v", got, def)
	}
}
