package host

import "testing"

func TestRemoteHost(t *testing.T) {
	for url, want := range map[string]string{
		"https://github.com/acme/app.git":                "github.com",
		"https://ann:s3cret@GitHub.com/acme/app":         "github.com",
		"git@github.com:acme/app.git":                    "github.com",
		"ssh://git@gitlab.example.com:2222/acme/app.git": "gitlab.example.com",
		"/srv/git/app.git":                               "",
		"../app.git":                                     "..",
	} {
		if got := remoteHost(url); got != want {
			t.Errorf("remoteHost(%q) = %q, want %q", url, got, want)
		}
	}
}
