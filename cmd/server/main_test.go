package main

import "testing"

func TestGetEnvIntOrDefault(t *testing.T) {
	const key = "ACOUSTICDNA_TEST_PORT"

	tests := []struct {
		name     string
		set      bool
		value    string
		fallback int
		want     int
	}{
		{name: "valid env overrides default", set: true, value: "9090", fallback: 8080, want: 9090},
		{name: "unset falls back to default", set: false, fallback: 8080, want: 8080},
		{name: "malformed value falls back to default", set: true, value: "abc", fallback: 8080, want: 8080},
		{name: "empty value falls back to default", set: true, value: "", fallback: 8080, want: 8080},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(key, tc.value)
			}
			// When tc.set is false we do not set the var; t.Setenv on other
			// subtests is scoped/restored per-test, so this reads unset.
			if got := getEnvIntOrDefault(key, tc.fallback); got != tc.want {
				t.Errorf("getEnvIntOrDefault(%q, %d) = %d, want %d", key, tc.fallback, got, tc.want)
			}
		})
	}
}
