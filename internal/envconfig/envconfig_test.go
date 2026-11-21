package envconfig

import (
	"log/slog"
	"os"
	"slices"
	"testing"
)

// setenv sets key to value for the test, or unsets it when unset is true.
// t.Setenv restores the previous state after the test in both cases.
func setenv(t *testing.T, key, value string, unset bool) {
	t.Helper()
	t.Setenv(key, value)
	if unset {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}
}

func TestList(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		unset   bool
		want    []string
		wantErr bool
	}{
		{
			name:    "ok",
			in:      "a:9091, b:9092",
			want:    []string{"a:9091", "b:9092"},
			wantErr: false,
		},
		{
			name:    "single addres",
			in:      "a:9091",
			want:    []string{"a:9091"},
			wantErr: false,
		},
		{
			name:    "spaces around items",
			in:      " a:9091 , b:9092 ",
			want:    []string{"a:9091", "b:9092"},
			wantErr: false,
		},
		{
			name:    "empty element",
			in:      "a:9091,,b:9092",
			want:    []string{"a:9091", "b:9092"},
			wantErr: false,
		},
		{
			name:    "empty second element",
			in:      "a:9091,",
			want:    []string{"a:9091"},
			wantErr: false,
		},
		{
			name:    "empty config",
			in:      "",
			want:    []string{},
			wantErr: true,
		},
		{
			name:    "only spaces",
			in:      " ",
			want:    []string{},
			wantErr: true,
		},
		{
			name:    "has no addresses",
			in:      ",",
			want:    []string{},
			wantErr: true,
		},
		{
			name:    "unset",
			unset:   true,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setenv(t, "TEST_LIST", tc.in, tc.unset)
			addrs, err := List("TEST_LIST")

			if (err != nil) != tc.wantErr {
				t.Errorf("List(%q) err = %v, wantErr %v", tc.in, err, tc.wantErr)
			}

			if err != nil {
				return
			}

			if !slices.Equal(addrs, tc.want) {
				t.Errorf("List(%q) got = %v, want %v", tc.in, addrs, tc.want)
			}
		})
	}
}

func TestLogLevel(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		unset   bool
		want    slog.Level
		wantErr bool
	}{
		{
			name:    "debug",
			in:      "debug",
			want:    slog.LevelDebug,
			wantErr: false,
		},
		{
			name:    "info",
			in:      "info",
			want:    slog.LevelInfo,
			wantErr: false,
		},
		{
			name:    "warn",
			in:      "warn",
			want:    slog.LevelWarn,
			wantErr: false,
		},
		{
			name:    "error",
			in:      "error",
			want:    slog.LevelError,
			wantErr: false,
		},
		{
			name:    "empty uses default",
			in:      "",
			want:    slog.LevelInfo,
			wantErr: false,
		},
		{
			name:    "unset uses default",
			unset:   true,
			want:    slog.LevelInfo,
			wantErr: false,
		},
		{
			name:    "upper case",
			in:      "DEBUG",
			wantErr: true,
		},
		{
			name:    "unknown level",
			in:      "verbose",
			wantErr: true,
		},
		{
			name:    "surrounding spaces",
			in:      " debug ",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setenv(t, "TEST_LOG_LEVEL", tc.in, tc.unset)
			lvl, err := logLevel("TEST_LOG_LEVEL")

			if (err != nil) != tc.wantErr {
				t.Errorf("logLevel(%q) err = %v, wantErr %v", tc.in, err, tc.wantErr)
			}

			if err != nil {
				return
			}

			if lvl != tc.want {
				t.Errorf("logLevel(%q) got = %v, want %v", tc.in, lvl, tc.want)
			}
		})
	}
}

func TestRequired(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		unset   bool
		want    string
		wantErr bool
	}{
		{name: "set", in: "user-events", want: "user-events"},
		{name: "kept as is", in: " user-events ", want: " user-events "},
		{name: "empty", in: "", wantErr: true},
		{name: "unset", unset: true, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setenv(t, "TEST_REQUIRED", tc.in, tc.unset)
			got, err := Required("TEST_REQUIRED")

			if (err != nil) != tc.wantErr {
				t.Fatalf("Required() err = %v, wantErr %v", err, tc.wantErr)
			}
			// The value returned with an error is not part of the contract.
			if err != nil {
				return
			}
			if got != tc.want {
				t.Errorf("Required() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOrDefault(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		unset bool
		want  string
	}{
		{name: "set", in: ":9090", want: ":9090"},
		{name: "empty", in: "", want: ":8080"},
		{name: "unset", unset: true, want: ":8080"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setenv(t, "TEST_OR_DEFAULT", tc.in, tc.unset)
			if got := OrDefault("TEST_OR_DEFAULT", ":8080"); got != tc.want {
				t.Errorf("OrDefault() = %q, want %q", got, tc.want)
			}
		})
	}
}
