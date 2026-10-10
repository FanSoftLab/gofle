package cmds

import (
	"GoFLE/pkg/config"
	"testing"
)

func TestTargetFromSavedTabItemLocal(t *testing.T) {
	got := targetFromSavedTabItem(config.SavedTabItem{Path: "/var/log/app.log"})
	if got != "/var/log/app.log" {
		t.Fatalf("got %q", got)
	}
}

func TestTargetFromSavedTabItemSSH(t *testing.T) {
	got := targetFromSavedTabItem(config.SavedTabItem{
		IsSSH: true,
		SSH: &config.SSHConfigSummary{
			Username:       "bob",
			Host:           "host.example",
			Port:           22,
			RemoteFilePath: "/var/log/x.log",
			Protocol:       "ssh",
		},
	})
	want := "ssh://bob@host.example:22/var/log/x.log"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestTargetFromSavedTabItemFTP(t *testing.T) {
	got := targetFromSavedTabItem(config.SavedTabItem{
		IsSSH: true,
		SSH: &config.SSHConfigSummary{
			Host:           "ftp.example",
			RemoteFilePath: "/logs/a.log",
			Protocol:       "ftp",
		},
	})
	want := "ftp://ftp.example/logs/a.log"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
