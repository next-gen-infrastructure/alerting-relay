// Package webhook defines the shape of Alertmanager's webhook_configs POST body.
package webhook

// Alert mirrors Alertmanager's webhook alert shape (subset used here).
type Alert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

// Payload mirrors Alertmanager's webhook_configs POST body (subset used here).
// Callers just send Alertmanager's default alert metadata — the relay owns formatting.
type Payload struct {
	Receiver          string            `json:"receiver"`
	Status            string            `json:"status"`
	GroupKey          string            `json:"groupKey"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	Alerts            []Alert           `json:"alerts"`
}
