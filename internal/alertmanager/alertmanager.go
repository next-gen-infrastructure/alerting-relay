// Package alertmanager reads inhibited alerts from the Alertmanager API:
// webhooks never carry them, so the relay has to ask.
package alertmanager

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"alerting-relay/internal/webhook"
)

type Client struct {
	baseURL        string
	user, password string
	http           *http.Client
}

// New builds a client; user/password enable basic auth when user is non-empty.
func New(baseURL, user, password string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), user: user, password: password, http: &http.Client{Timeout: 10 * time.Second}}
}

// matchKeys picks the labels that identify a webhook group's scope. Workload
// groups span several alertnames, so alertname is only used without a workload.
func matchKeys(labels map[string]string) []string {
	if labels["workload"] != "" {
		return []string{"cluster", "namespace", "workload"}
	}
	return []string{"cluster", "namespace", "alertname"}
}

// Inhibited returns the currently inhibited alerts in the same scope as
// labels (a webhook payload's CommonLabels), sorted by fingerprint.
// Alerts get Status "inhibited" so they render apart from firing ones.
func (c *Client) Inhibited(labels map[string]string) ([]webhook.Alert, error) {
	q := url.Values{"inhibited": {"true"}, "active": {"false"}, "silenced": {"false"}, "unprocessed": {"false"}}
	for _, k := range matchKeys(labels) {
		if v := labels[k]; v != "" {
			q.Add("filter", fmt.Sprintf("%s=%q", k, v))
		}
	}
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/v2/alerts?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if c.user != "" {
		req.SetBasicAuth(c.user, c.password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alertmanager: %s", resp.Status)
	}
	var raw []struct {
		Fingerprint string            `json:"fingerprint"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]webhook.Alert, len(raw))
	for i, a := range raw {
		out[i] = webhook.Alert{Status: "inhibited", Labels: a.Labels, Annotations: a.Annotations, Fingerprint: a.Fingerprint}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Fingerprint < out[j].Fingerprint })
	return out, nil
}
