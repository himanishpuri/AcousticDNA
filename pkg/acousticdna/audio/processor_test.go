package audio

import (
	"slices"
	"testing"
)

func TestYtdlpCommonArgsCleanHost(t *testing.T) {
	// No env vars set: none of the machine-specific flags should appear.
	for _, k := range []string{
		"YTDLP_COOKIES_FROM_BROWSER",
		"YTDLP_JS_RUNTIMES",
		"YTDLP_REMOTE_COMPONENTS",
		"YTDLP_EXTRA_ARGS",
	} {
		t.Setenv(k, "")
	}

	args := ytdlpCommonArgs()
	want := []string{"--no-warnings", "--no-playlist"}
	if !slices.Equal(args, want) {
		t.Fatalf("clean host args = %v, want %v", args, want)
	}
	for _, forbidden := range []string{"--cookies-from-browser", "--js-runtimes", "--remote-components"} {
		if slices.Contains(args, forbidden) {
			t.Errorf("clean host args unexpectedly contain %q", forbidden)
		}
	}
}

func TestPickArtist(t *testing.T) {
	tests := []struct {
		name string
		meta YTMetadata
		want string
	}{
		{"artist set", YTMetadata{Artist: "Radiohead", Channel: "chan", Uploader: "up"}, "Radiohead"},
		// Whitespace-only Artist is treated as empty and falls back to Channel.
		// (This differs from an exact `Artist == ""` check; documented behavior.)
		{"whitespace artist falls back to channel", YTMetadata{Artist: "   ", Channel: "SomeChannel"}, "SomeChannel"},
		{"empty artist falls back to channel", YTMetadata{Channel: "SomeChannel", Uploader: "up"}, "SomeChannel"},
		{"falls back to uploader", YTMetadata{Uploader: "SomeUploader"}, "SomeUploader"},
		{"all empty", YTMetadata{}, "Unknown Artist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickArtist(tt.meta); got != tt.want {
				t.Errorf("pickArtist(%+v) = %q, want %q", tt.meta, got, tt.want)
			}
		})
	}
}

func TestYtdlpCommonArgsOptIn(t *testing.T) {
	t.Setenv("YTDLP_COOKIES_FROM_BROWSER", "firefox")
	t.Setenv("YTDLP_JS_RUNTIMES", "deno")
	t.Setenv("YTDLP_REMOTE_COMPONENTS", "ejs:github")
	t.Setenv("YTDLP_EXTRA_ARGS", "--sleep-requests 1")

	args := ytdlpCommonArgs()
	want := []string{
		"--no-warnings", "--no-playlist",
		"--cookies-from-browser", "firefox",
		"--js-runtimes", "deno",
		"--remote-components", "ejs:github",
		"--sleep-requests", "1",
	}
	if !slices.Equal(args, want) {
		t.Fatalf("opt-in args = %v, want %v", args, want)
	}
}
