package store

import (
	"testing"
	"time"
)

func TestSQLiteLifecycle(t *testing.T) {
	s, err := New("sqlite://:memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if ag, err := s.Get("r", "g"); err != nil || ag != nil {
		t.Fatalf("Get on empty store = %v, %v; want nil, nil", ag, err)
	}

	if err := s.Create(AlertGroup{Receiver: "r", GroupKey: "g", Channel: "c", MessageTS: "1", Status: "firing"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	ag, err := s.Get("r", "g")
	if err != nil || ag == nil || ag.Status != "firing" {
		t.Fatalf("Get after Create = %+v, %v", ag, err)
	}

	if err := s.Touch("r", "g"); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	if err := s.MarkResolved("r", "g"); err != nil {
		t.Fatalf("MarkResolved: %v", err)
	}
	ag, err = s.Get("r", "g")
	if err != nil || ag == nil || ag.Status != "resolved" || ag.ResolvedAt == nil {
		t.Fatalf("Get after MarkResolved = %+v, %v", ag, err)
	}

	n, err := s.DeleteResolvedOlderThan(time.Hour)
	if err != nil || n != 0 {
		t.Fatalf("DeleteResolvedOlderThan(1h) = %d, %v; want 0, nil", n, err)
	}

	if _, err := s.db.Exec(`UPDATE alert_groups SET resolved_at = datetime('now', '-2 hours')`); err != nil {
		t.Fatalf("backdating resolved_at: %v", err)
	}
	n, err = s.DeleteResolvedOlderThan(time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("DeleteResolvedOlderThan(1h) after backdating = %d, %v; want 1, nil", n, err)
	}
}

func TestFiringScopesAndInhibitedDedup(t *testing.T) {
	s, err := New("sqlite://:memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Create(AlertGroup{Receiver: "r", GroupKey: "g", Channel: "C", MessageTS: "1", Status: "firing"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveScope("r", "g", map[string]string{"workload": "api"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListFiring()
	if err != nil || len(got) != 1 || got[0].Labels["workload"] != "api" {
		t.Fatalf("ListFiring = %#v, %v", got, err)
	}
	if fresh, _ := s.MarkInhibitedPosted("r", "g", []string{"a", "b"}); len(fresh) != 2 {
		t.Fatalf("expected 2 fresh, got %v", fresh)
	}
	if fresh, _ := s.MarkInhibitedPosted("r", "g", []string{"b", "c"}); len(fresh) != 1 || fresh[0] != "c" {
		t.Fatalf("expected only c fresh, got %v", fresh)
	}
	if err := s.MarkResolved("r", "g"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListFiring(); len(got) != 0 {
		t.Fatalf("resolved group must not be polled, got %#v", got)
	}
}
