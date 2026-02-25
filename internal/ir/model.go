package ir

type PlanIR struct {
	IRVersion                       string     `json:"ir_version"`
	DeterminismPolicyVersion        string     `json:"determinism_policy_version"`
	SemanticDerivationPolicyVersion string     `json:"semantic_derivation_policy_version"`
	PlanPath                        string     `json:"plan_path"`
	Source                          SourceIR   `json:"source"`
	Semantic                        SemanticIR `json:"semantic"`
}

type SourceIR struct {
	Nodes []SourceNode `json:"nodes"`
}

type SourceNode struct {
	NodeRef    string `json:"node_ref"`
	Kind       string `json:"kind"`
	Line       int    `json:"line"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	SliceHash  string `json:"slice_hash"`
	SliceText  string `json:"slice_text"`
	Text       string `json:"text"`
	Checked    bool   `json:"checked,omitempty"`
	MetaOpaque []Meta `json:"meta_opaque,omitempty"`
}

type SemanticIR struct {
	Tasks []Task `json:"tasks"`
}

type Task struct {
	ID      string   `json:"id"`
	NodeRef string   `json:"node_ref"`
	Title   string   `json:"title"`
	Horizon string   `json:"horizon,omitempty"`
	Deps    []string `json:"deps,omitempty"`
	Accept  []string `json:"accept,omitempty"`
}

type Meta struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Line  int    `json:"line"`
}
