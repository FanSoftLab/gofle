package cmds

import (
	"GoFLE/pkg/config"
	"testing"
)

func TestDescribeRecentLocal(t *testing.T) {
	got := describeRecent("/var/log/x.log", false, nil)
	if got != "/var/log/x.log" {
		t.Fatalf("expected local path, got %q", got)
	}
}

func TestDescribeRecentSSH(t *testing.T) {
	ssh := &config.SSHConfigSummary{
		Host:           "host.example.com",
		Port:           2222,
		Username:       "bob",
		RemoteFilePath: "/var/log/sys.log",
	}
	got := describeRecent("ssh://bob@host.example.com:2222/var/log/sys.log", true, ssh)
	// SSH entries are displayed WITHOUT the username (and never credentials):
	// connection details stay in the encrypted summary.
	want := "host.example.com:2222/var/log/sys.log"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildSSHURL(t *testing.T) {
	cases := []struct {
		user, host, path string
		port             int
		want             string
	}{
		{"", "h", "/a", 22, "ssh://h:22/a"},
		{"u", "h", "/a", 22, "ssh://h:22/a"},
		{"u", "h", "/b/c", 2022, "ssh://h:2022/b/c"},
	}
	for _, c := range cases {
		if got := buildSSHURL(c.user, c.host, c.port, c.path); got != c.want {
			t.Fatalf("buildSSHURL(%q,%q,%d,%q)=%q want %q", c.user, c.host, c.port, c.path, got, c.want)
		}
	}
}
