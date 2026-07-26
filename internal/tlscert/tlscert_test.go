package tlscert

import (
	"context"
	"testing"
)

type fakeSettingsStore struct {
	values map[string]string
}

func newFakeSettingsStore() *fakeSettingsStore {
	return &fakeSettingsStore{values: map[string]string{}}
}

func (f *fakeSettingsStore) GetSetting(_ context.Context, key string) (string, bool, error) {
	v, ok := f.values[key]
	return v, ok, nil
}

func (f *fakeSettingsStore) SetSetting(_ context.Context, key, value string) error {
	f.values[key] = value
	return nil
}

func TestLoadOrGenerateCreatesAndCaches(t *testing.T) {
	ctx := context.Background()
	s := newFakeSettingsStore()

	if len(s.values) != 0 {
		t.Fatalf("fake store should start empty")
	}

	cert1, err := LoadOrGenerate(ctx, s)
	if err != nil {
		t.Fatalf("LoadOrGenerate: %v", err)
	}
	if len(cert1.Certificate) == 0 {
		t.Fatal("generated certificate has no DER bytes")
	}
	if s.values[settingCert] == "" || s.values[settingKey] == "" {
		t.Fatal("cert/key were not persisted to settings")
	}

	cert2, err := LoadOrGenerate(ctx, s)
	if err != nil {
		t.Fatalf("LoadOrGenerate (second call): %v", err)
	}
	if string(cert1.Certificate[0]) != string(cert2.Certificate[0]) {
		t.Fatal("second call generated a new certificate instead of reusing the cached one")
	}
}

func TestLoadOrGenerateRegeneratesOnCorruptSettings(t *testing.T) {
	ctx := context.Background()
	s := newFakeSettingsStore()
	s.values[settingCert] = "not a valid pem"
	s.values[settingKey] = "also not valid"

	cert, err := LoadOrGenerate(ctx, s)
	if err != nil {
		t.Fatalf("LoadOrGenerate should recover from corrupt cached values: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("expected a freshly generated certificate")
	}
}
