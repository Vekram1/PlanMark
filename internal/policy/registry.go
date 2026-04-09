package policy

import (
	"fmt"
	"sort"
)

type Kind string

const (
	KindDeterminism        Kind = "determinism"
	KindSemanticDerivation Kind = "semantic_derivation"
	KindChangeDetection    Kind = "change_detection"
	KindTrackerReconcile   Kind = "tracker_reconcile"
)

type Registry struct {
	supported map[Kind][]string
	defaults  map[Kind]string
}

func NewRegistry() *Registry {
	return &Registry{
		supported: map[Kind][]string{
			KindDeterminism:        {"v0.1"},
			KindSemanticDerivation: {"v0.1", "v0.2", "v0.3", "v0.4"},
			KindChangeDetection:    {"v0.1"},
			KindTrackerReconcile:   {"v0.1"},
		},
		defaults: map[Kind]string{
			KindDeterminism:        "v0.1",
			KindSemanticDerivation: "v0.4",
			KindChangeDetection:    "v0.1",
			KindTrackerReconcile:   "v0.1",
		},
	}
}

func (r *Registry) Select(kind Kind, explicitVersion string) (string, error) {
	versions := r.supported[kind]
	if len(versions) == 0 {
		return "", fmt.Errorf("unsupported policy kind: %s", kind)
	}
	if explicitVersion == "" {
		return r.defaults[kind], nil
	}
	for _, v := range versions {
		if v == explicitVersion {
			return explicitVersion, nil
		}
	}
	return "", fmt.Errorf("unsupported policy version %q for kind %s", explicitVersion, kind)
}

func (r *Registry) SupportedVersions() map[Kind][]string {
	out := make(map[Kind][]string, len(r.supported))
	for kind, versions := range r.supported {
		cp := append([]string(nil), versions...)
		sort.Strings(cp)
		out[kind] = cp
	}
	return out
}
