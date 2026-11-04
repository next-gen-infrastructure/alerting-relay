package alertmanager

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInhibited(t *testing.T) {
	var gotFilters []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFilters = r.URL.Query()["filter"]
		_, _ = w.Write([]byte(`[{"fingerprint":"b","labels":{"alertname":"KubePodCrashLooping"},"annotations":{"description":"d"}},{"fingerprint":"a","labels":{}}]`))
	}))
	defer srv.Close()

	got, err := New(srv.URL, "", "").Inhibited(map[string]string{"cluster": "c", "namespace": "ns", "workload": "api", "alertname": "X"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Fingerprint != "a" || got[1].Status != "inhibited" {
		t.Fatalf("unexpected alerts: %#v", got)
	}
	if len(gotFilters) != 3 || gotFilters[2] != `workload="api"` {
		t.Fatalf("expected cluster/namespace/workload filters, got %v", gotFilters)
	}
}
