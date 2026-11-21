package models

// RepositoryGovernanceOptions selects a view over one shared repository snapshot.
type RepositoryGovernanceOptions struct {
	View      string `json:"view"`
	Ownership string `json:"ownership"`
}

type RepositoryGovernanceQuery struct {
	Query
	Options RepositoryGovernanceOptions `json:"options"`
}
