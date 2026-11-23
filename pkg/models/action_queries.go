package models

type WorkflowRunOptions struct {
	RunID   string `json:"runId"`
	Attempt string `json:"attempt"`
}

type WorkflowRunQuery struct {
	Query
	Options WorkflowRunOptions `json:"options"`
}

type WorkflowFailuresOptions struct {
	Repositories []string `json:"repositories"`
	MaxRuns      int      `json:"maxRuns"`
	Branch       string   `json:"branch"`
	View         string   `json:"view"`
}

type WorkflowFailuresQuery struct {
	Query
	Options WorkflowFailuresOptions `json:"options"`
}

type ActionInventoryOptions struct {
	Repositories        []string `json:"repositories"`
	Ref                 string   `json:"ref"`
	View                string   `json:"view"`
	ResolveDependencies bool     `json:"resolveDependencies"`
}

type ActionInventoryQuery struct {
	Query
	Options ActionInventoryOptions `json:"options"`
}
