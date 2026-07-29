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
