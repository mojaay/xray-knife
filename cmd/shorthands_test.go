package cmd

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// TestShorthandsAreCanonical asserts every shorthand in the command tree binds
// to the one long name canonicalShorthands assigns it. Known violations are
// listed in grandfathered and are removed as each command is realigned.
func TestShorthandsAreCanonical(t *testing.T) {
	var violations []string

	visitShorthands(rootCmd, func(path string, f *pflag.Flag) {
		key := fmt.Sprintf("%s -%s", path, f.Shorthand)
		want, registered := canonicalShorthands[f.Shorthand]
		ok := registered && want == f.Name
		if ok {
			if grandfathered[key] {
				t.Errorf("%s now binds --%s correctly; delete it from grandfathered in cmd/shorthands.go", key, f.Name)
			}
			return
		}
		if grandfathered[key] {
			return
		}
		if !registered {
			violations = append(violations, fmt.Sprintf("%s binds --%s but -%s is not in canonicalShorthands", key, f.Name, f.Shorthand))
			return
		}
		violations = append(violations, fmt.Sprintf("%s binds --%s but -%s is reserved for --%s", key, f.Name, f.Shorthand, want))
	})

	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("shorthand violations:\n  %s", strings.Join(violations, "\n  "))
	}
}

// TestOneNameOneShorthand asserts a given long flag name never carries two
// different shorthands across the tree (catches --out/--output style drift).
func TestOneNameOneShorthand(t *testing.T) {
	seen := map[string]string{} // long name -> shorthand
	where := map[string]string{}

	visitShorthands(rootCmd, func(path string, f *pflag.Flag) {
		if prev, ok := seen[f.Name]; ok && prev != f.Shorthand {
			t.Errorf("--%s is -%s in %s but -%s in %s", f.Name, prev, where[f.Name], f.Shorthand, path)
			return
		}
		seen[f.Name] = f.Shorthand
		where[f.Name] = path
	})
}

func TestFlagErrorFuncExplainsRemovedShorthand(t *testing.T) {
	err := flagErrorFunc(rootCmd, errors.New("unknown shorthand flag: 'k' in -k"))
	msg := err.Error()

	if !strings.Contains(msg, "--only-speedtest") {
		t.Errorf("hint for -k should name --only-speedtest, got: %s", msg)
	}
	if !strings.Contains(msg, "v11") {
		t.Errorf("hint should mention the v11 realignment, got: %s", msg)
	}
}

func TestFlagErrorFuncPassesThroughUnknownLetters(t *testing.T) {
	orig := errors.New("unknown shorthand flag: 'Q' in -Q")
	if got := flagErrorFunc(rootCmd, orig); got.Error() != orig.Error() {
		t.Errorf("unmapped letter should pass through unchanged, got: %s", got)
	}
}

func TestEveryRemovedShorthandNamesALongFlag(t *testing.T) {
	for letter, hint := range removedShorthands {
		if !strings.Contains(hint, "--") {
			t.Errorf("removedShorthands[%q] = %q: must name the replacement long flag", letter, hint)
		}
	}
}

func TestResolveVersionPrefersLdflags(t *testing.T) {
	saved := version
	t.Cleanup(func() { version = saved })

	version = "11.2.3"
	if got := resolveVersion(); got != "11.2.3" {
		t.Fatalf("resolveVersion() = %q, want %q", got, "11.2.3")
	}
}

func TestResolveVersionNeverEmpty(t *testing.T) {
	saved := version
	t.Cleanup(func() { version = saved })

	version = ""
	if got := resolveVersion(); got == "" {
		t.Fatal("resolveVersion() returned an empty string")
	}
}
