package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/simjay/plous/internal/app/activity"
	"github.com/simjay/plous/internal/app/monitor"
)

func TestMonitorsOpenOnlySelectedAlertsAndRetainChatDraft(t *testing.T) {
	m := activityTimelineModel(t)
	m.width, m.height = 120, 28
	m.mode, m.focus = 1, 2
	m.draft().Text = "unfinished chat draft"
	now := time.Now()
	checkout := monitor.State{
		Name: "checkout", Context: "fixture", Namespace: "payments", Running: true,
		Interval: 5 * time.Minute, LastOK: now,
		Alerts: []monitor.Alert{{
			ID: "checkout-alert", Name: "checkout", Kind: "DaemonSet", State: "open",
			Signal: "low availability", Detail: "8 of 12 eligible nodes available", FirstSeen: now, LastSeen: now,
		}},
	}
	stopped := monitor.State{
		Name: "checkout-stopped", Context: "other", Namespace: "payments",
		Alerts: []monitor.Alert{{
			ID: "historical-alert", Name: "checkout-stopped", Kind: "Deployment", State: "open",
			Signal: "stalled rollout", FirstSeen: now, LastSeen: now,
		}},
	}
	m.Update(monitorStatusMsg{states: []monitor.State{checkout, stopped}})
	if m.focus != 2 || m.mode != 1 || !strings.Contains(ansi.Strip(m.render()), "1 open alert") {
		t.Fatal("the running monitor did not update the header in place")
	}
	clickMouse(t, m, "header", "monitors")
	if m.page != monitorsPage || m.draft().Text != "unfinished chat draft" {
		t.Fatal("header did not open Monitors while retaining the draft")
	}
	screen := ansi.Strip(m.render())
	if !strings.Contains(screen, "fixture/payments") || strings.Contains(screen, "checkout-stopped") {
		t.Fatalf("expected only the running monitor:\n%s", screen)
	}

	press(t, m, "enter")
	entries := m.activityEntries()
	if m.mode != 2 || m.activityUI.Filter != activity.Alerts || len(entries) != 1 || entries[0].MonitorName != "checkout" {
		t.Fatalf("expected only the selected monitor's alerts, got %+v", entries)
	}
	press(t, m, "enter")
	if !strings.Contains(m.detail, "8 of 12 eligible nodes available") {
		t.Fatalf("alert evidence not shown: %q", m.detail)
	}
	press(t, m, "esc")
	press(t, m, "esc")
	if m.page != monitorsPage {
		t.Fatal("Esc from the monitor's alerts did not return to Monitors")
	}
	m.execute("j")
	if len(m.activityEntries()) != 2 {
		t.Fatal("global Alerts should include saved alerts from stopped monitors")
	}
}

func TestMonitorsShowsRetryScheduleAndOverdueCheck(t *testing.T) {
	now := time.Now()
	backoff := monitor.State{
		Name: "retrying", Running: true, Interval: time.Minute,
		LastCheck: now.Add(-2 * time.Minute), NextCheck: now.Add(2 * time.Minute),
	}
	overdue := monitor.State{Name: "late", Running: true, NextCheck: now.Add(-time.Minute)}
	m := model{monitorStates: []monitor.State{backoff, overdue}}
	for _, width := range []int{20, 100} {
		m.monitorSelection = "retrying"
		body := ansi.Strip(m.monitorsBody(width, 10))
		if !strings.Contains(body, "Next check ") || !strings.Contains(body, "Overdue ") {
			t.Fatalf("width %d did not show both schedules:\n%s", width, body)
		}
	}
	line := ansi.Strip(monitorScheduleLine(backoff, now, 20, true))
	if line != "Next check "+backoff.NextCheck.Local().Format("15:04:05") {
		t.Fatalf("retry used the wrong due time: %q", line)
	}
	overdue.NextCheck = now.Add(-time.Second)
	if got := ansi.Strip(monitorScheduleLine(overdue, now, 20, true)); !strings.HasPrefix(got, "Check due ") {
		t.Fatalf("a check within its deadline was mislabeled: %q", got)
	}
	overdue.NextCheck = time.Time{}
	if got := ansi.Strip(monitorScheduleLine(overdue, now, 20, true)); got != "Next check unknown" {
		t.Fatalf("a record with no schedule was mislabeled: %q", got)
	}
}
