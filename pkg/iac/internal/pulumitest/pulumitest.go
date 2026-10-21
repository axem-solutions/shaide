// Package pulumitest runs a Pulumi program against mocks and records every
// resource it registers, with inputs reduced to plain Go values. It is for
// tests only.
package pulumitest

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Resource is one registered resource.
type Resource struct {
	Type   string
	Name   string
	Inputs map[string]any
	// Secrets lists input paths that were marked secret.
	Secrets []string
}

// Result holds what a program registered.
type Result struct {
	Resources []Resource
}

type monitor struct {
	mu        sync.Mutex
	resources []Resource
}

func (m *monitor) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	var secrets []string
	inputs, _ := plain(resource.NewObjectProperty(args.Inputs), "", &secrets).(map[string]any)
	m.mu.Lock()
	m.resources = append(m.resources, Resource{Type: args.TypeToken, Name: args.Name, Inputs: inputs, Secrets: secrets})
	m.mu.Unlock()
	return args.Name + "-id", args.Inputs, nil
}

func (m *monitor) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

// plain converts a property value to Go values, recording secret paths.
func plain(v resource.PropertyValue, path string, secrets *[]string) any {
	switch {
	case v.IsSecret():
		*secrets = append(*secrets, path)
		return plain(v.SecretValue().Element, path, secrets)
	case v.IsOutput():
		out := v.OutputValue()
		if out.Secret {
			*secrets = append(*secrets, path)
		}
		if !out.Known {
			return nil
		}
		return plain(out.Element, path, secrets)
	case v.IsComputed():
		return nil
	case v.IsObject():
		obj := map[string]any{}
		for key, value := range v.ObjectValue() {
			obj[string(key)] = plain(value, path+"."+string(key), secrets)
		}
		return obj
	case v.IsArray():
		arr := []any{}
		for i, value := range v.ArrayValue() {
			arr = append(arr, plain(value, fmt.Sprintf("%s[%d]", path, i), secrets))
		}
		return arr
	case v.IsNull():
		return nil
	default:
		return v.V
	}
}

// Run executes program with config set as stack configuration and returns
// the registered resources, or the program's error.
func Run(t *testing.T, project string, config auto.ConfigMap, program pulumi.RunFunc) (Result, error) {
	t.Helper()

	values := map[string]string{}
	var secretKeys []string
	for key, value := range config {
		values[key] = value.Value
		if value.Secret {
			secretKeys = append(secretKeys, key)
		}
	}
	sort.Strings(secretKeys)
	encoded, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("encode config: %v", err)
	}
	encodedSecrets, err := json.Marshal(secretKeys)
	if err != nil {
		t.Fatalf("encode secret keys: %v", err)
	}
	t.Setenv("PULUMI_CONFIG", string(encoded))
	t.Setenv("PULUMI_CONFIG_SECRET_KEYS", string(encodedSecrets))

	mocks := &monitor{}
	err = pulumi.RunErr(program, pulumi.WithMocks(project, "test", mocks))

	mocks.mu.Lock()
	defer mocks.mu.Unlock()
	return Result{Resources: append([]Resource(nil), mocks.resources...)}, err
}

// Find returns the resource of the given type token and Pulumi name.
func (r Result) Find(typ, name string) (Resource, bool) {
	for _, res := range r.Resources {
		if res.Type == typ && res.Name == name {
			return res, true
		}
	}
	return Resource{}, false
}

// Must is Find that fails the test when the resource is absent.
func (r Result) Must(t *testing.T, typ, name string) Resource {
	t.Helper()
	res, ok := r.Find(typ, name)
	if !ok {
		t.Fatalf("no %s %q registered; have %s", typ, name, r.names())
	}
	return res
}

// OfType returns every resource of a type token.
func (r Result) OfType(typ string) []Resource {
	var out []Resource
	for _, res := range r.Resources {
		if res.Type == typ {
			out = append(out, res)
		}
	}
	return out
}

func (r Result) names() string {
	var names []string
	for _, res := range r.Resources {
		names = append(names, res.Type+"/"+res.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// Get walks a dotted path ("spec.template.spec.containers") through maps.
// Array elements are addressed by number ("containers.0.env").
func Get(value any, path string) any {
	current := value
	for _, part := range strings.Split(path, ".") {
		switch node := current.(type) {
		case map[string]any:
			current = node[part]
		case []any:
			var index int
			if _, err := fmt.Sscanf(part, "%d", &index); err != nil || index < 0 || index >= len(node) {
				return nil
			}
			current = node[index]
		default:
			return nil
		}
	}
	return current
}

// String is Get for a string value; empty when absent.
func String(value any, path string) string {
	s, _ := Get(value, path).(string)
	return s
}

// List is Get for a list value.
func List(value any, path string) []any {
	l, _ := Get(value, path).([]any)
	return l
}

// Map is Get for a map value.
func Map(value any, path string) map[string]any {
	m, _ := Get(value, path).(map[string]any)
	return m
}

// Named returns the element of a list of objects whose "name" is name.
func Named(list []any, name string) map[string]any {
	for _, item := range list {
		if m, ok := item.(map[string]any); ok && m["name"] == name {
			return m
		}
	}
	return nil
}

// Env flattens a container env list to name -> value, and name ->
// "secret:<name>/<key>" or "configmap:<name>/<key>" for references.
func Env(container map[string]any) map[string]string {
	out := map[string]string{}
	for _, item := range List(container, "env") {
		env, _ := item.(map[string]any)
		name, _ := env["name"].(string)
		switch {
		case Get(env, "valueFrom.secretKeyRef") != nil:
			out[name] = "secret:" + String(env, "valueFrom.secretKeyRef.name") + "/" + String(env, "valueFrom.secretKeyRef.key")
		case Get(env, "valueFrom.configMapKeyRef") != nil:
			out[name] = "configmap:" + String(env, "valueFrom.configMapKeyRef.name") + "/" + String(env, "valueFrom.configMapKeyRef.key")
		default:
			out[name], _ = env["value"].(string)
		}
	}
	return out
}
