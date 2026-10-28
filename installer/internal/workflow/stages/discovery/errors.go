package discovery

import (
	"fmt"
	"strings"
)

type harborResourceKind string

const (
	harborNamespace harborResourceKind = "namespace"
	harborService   harborResourceKind = "service"
	harborSecret    harborResourceKind = "secret"
)

type harborResourceProblem string

const (
	missing harborResourceProblem = "missing"
	invalid harborResourceProblem = "invalid"
)

type harborDiscoveryError struct {
	Kind         harborResourceKind
	Problem      harborResourceProblem
	Namespace    string
	ResourceName string
	// Candidates lists other namespaces that run a Harbor, for a missing
	// namespace.
	Candidates []string
	Err        error
}

func (e harborDiscoveryError) Error() string {
	target := e.ResourceName
	if e.Namespace != "" {
		target = fmt.Sprintf("%s/%s", e.Namespace, e.ResourceName)
	}

	msg := fmt.Sprintf("%s %s is %s", e.Kind, target, e.Problem)
	if len(e.Candidates) > 0 {
		msg += fmt.Sprintf("; Harbor found in namespace(s) %s", strings.Join(e.Candidates, ", "))
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", msg, e.Err)
	}

	return msg
}

func (e harborDiscoveryError) Unwrap() error {
	return e.Err
}
