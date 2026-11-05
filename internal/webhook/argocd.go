package webhook

// ArgoEvent is the JSON body rendered by an Argo CD notifications `service.webhook` template.
// Event is one of: syncing, synced, sync-failed, degraded, healthy.
type ArgoEvent struct {
	App       string `json:"app"`
	Cluster   string `json:"cluster"` // destination cluster; routes via SLACK_CHANNELS like the alert `cluster` label
	Namespace string `json:"namespace"`
	Revision  string `json:"revision"`
	Event     string `json:"event"`
	Message   string `json:"message"`
	Repo      string `json:"repo"` // app source repoURL
	URL       string `json:"url"`  // link to the app in the Argo CD UI
}
