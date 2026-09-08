package cmd

import "testing"

func TestResolveDockerHost(t *testing.T) {
	exists := func(known map[string]bool) func(string) bool {
		return func(path string) bool { return known[path] }
	}

	cases := []struct {
		name       string
		explicit   string
		env        string
		candidates []string
		known      map[string]bool
		wantHost   string
		wantSource string
	}{
		{
			name:       "explicit flag wins over everything",
			explicit:   "tcp://192.168.1.10:2375",
			env:        "unix:///env/docker.sock",
			candidates: []string{"/a/docker.sock"},
			known:      map[string]bool{"/a/docker.sock": true},
			wantHost:   "tcp://192.168.1.10:2375",
			wantSource: "flag",
		},
		{
			name:       "DOCKER_HOST env wins over probing",
			explicit:   "",
			env:        "unix:///env/docker.sock",
			candidates: []string{"/a/docker.sock"},
			known:      map[string]bool{"/a/docker.sock": true},
			wantHost:   "unix:///env/docker.sock",
			wantSource: "env DOCKER_HOST",
		},
		{
			name:       "first existing candidate wins",
			explicit:   "",
			env:        "",
			candidates: []string{"/missing/docker.sock", "/found/docker.sock", "/also/found.sock"},
			known:      map[string]bool{"/found/docker.sock": true, "/also/found.sock": true},
			wantHost:   "unix:///found/docker.sock",
			wantSource: "auto-detected",
		},
		{
			name:       "no candidate exists falls back to hardcoded default",
			explicit:   "",
			env:        "",
			candidates: []string{"/missing/docker.sock"},
			known:      map[string]bool{},
			wantHost:   defaultDockerHost,
			wantSource: "default",
		},
		{
			name:       "no candidates at all falls back to hardcoded default",
			explicit:   "",
			env:        "",
			candidates: nil,
			known:      map[string]bool{},
			wantHost:   defaultDockerHost,
			wantSource: "default",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveDockerHost(tc.explicit, tc.env, tc.candidates, exists(tc.known))
			if got.Host != tc.wantHost {
				t.Errorf("Host = %q, want %q", got.Host, tc.wantHost)
			}
			if got.Source != tc.wantSource {
				t.Errorf("Source = %q, want %q", got.Source, tc.wantSource)
			}
		})
	}
}

func TestCandidateDockerSocketsIncludesHomeAndXDG(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")

	candidates := candidateDockerSockets()

	found := map[string]bool{}
	for _, c := range candidates {
		found[c] = true
	}
	if !found["/var/run/docker.sock"] {
		t.Errorf("candidates %v missing /var/run/docker.sock", candidates)
	}
	if !found["/run/user/1000/docker.sock"] {
		t.Errorf("candidates %v missing XDG_RUNTIME_DIR-derived path", candidates)
	}
}

func TestDockerHostBannerNote(t *testing.T) {
	cases := map[string]string{
		"flag":            "",
		"default":         "",
		"env DOCKER_HOST": " (from $DOCKER_HOST)",
		"auto-detected":   " (auto-detected)",
	}
	for source, want := range cases {
		if got := dockerHostBannerNote(source); got != want {
			t.Errorf("dockerHostBannerNote(%q) = %q, want %q", source, got, want)
		}
	}
}
