package monitor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMonitorDirectoriesAndEarlierRecord(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"payments", "node-logs", "default"} {
		dir, err := directory(root, name)
		if err != nil || dir != filepath.Join(root, name) {
			t.Fatalf("directory for %s: %q (%v)", name, dir, err)
		}
	}
	if _, err := directory(root, "../elsewhere"); err == nil {
		t.Fatal("a monitor name could escape its storage directory")
	}

	for _, name := range []string{"", "payments"} {
		dir := root
		if name != "" {
			dir = filepath.Join(root, name)
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
		}
		lock, err := lockMonitor(dir, name)
		if err != nil {
			t.Fatal(err)
		}
		lock.Close()
		if name == "" {
			err = os.WriteFile(filepath.Join(dir, "watch.json"), []byte(`{"Context":"prod","Namespace":"shop","Running":true,"Incidents":[{"ID":"saved-alert","State":"open"}]}`), 0600)
		} else {
			err = save(dir, storedState{
				State:      State{Name: name, Context: "prod", Namespace: "shop", Interval: 3 * time.Minute},
				Kubeconfig: "/fixture/kubeconfig",
			})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	states, err := List(root)
	if err != nil || len(states) != 2 || states[0].Name != "default" || states[1].Name != "payments" {
		t.Fatalf("named and earlier records: %+v (%v)", states, err)
	}
	if earlier := states[0]; earlier.Running || earlier.OpenAlerts() != 1 || earlier.Alerts[0].ID != "saved-alert" {
		t.Fatalf("legacy alert or stopped status changed: %+v", earlier)
	}
	saved, err := readStored(filepath.Join(root, "payments"))
	if err != nil || saved.Name != "payments" || saved.Kubeconfig != "/fixture/kubeconfig" || saved.Namespace != "shop" || saved.Interval != 3*time.Minute {
		t.Fatalf("restart configuration: %+v (%v)", saved, err)
	}
	if err := remove(root, "default", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	states, err = List(root)
	if err != nil || len(states) != 1 || states[0].Name != "payments" {
		t.Fatalf("removing the earlier record must preserve its neighbor: %+v (%v)", states, err)
	}
}
