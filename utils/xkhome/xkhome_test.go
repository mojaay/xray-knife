package xkhome

import (
	"path/filepath"
	"testing"
)

func TestDirPrefersEnvVar(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XRAY_KNIFE_HOME", tmp)

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error = %v", err)
	}
	if got != tmp {
		t.Fatalf("Dir() = %q, want %q", got, tmp)
	}
}

func TestDBPathOverrideWins(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XRAY_KNIFE_HOME", tmp)

	want := filepath.Join(tmp, "custom.db")
	got, err := DBPath(want)
	if err != nil {
		t.Fatalf("DBPath() error = %v", err)
	}
	if got != want {
		t.Fatalf("DBPath(%q) = %q, want %q", want, got, want)
	}
}

func TestDBPathDefaultsUnderHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XRAY_KNIFE_HOME", tmp)

	want := filepath.Join(tmp, "xray-knife.db")
	got, err := DBPath("")
	if err != nil {
		t.Fatalf("DBPath() error = %v", err)
	}
	if got != want {
		t.Fatalf("DBPath(\"\") = %q, want %q", got, want)
	}
}
