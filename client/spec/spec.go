package spec

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
)

// CloudQuery Kubernetes source plugin config spec.
type Spec struct {
	// Specify K8s contexts to connect to.
	// Specifying `*` will connect to all contexts available in the K8s config file (usually `~/.kube/config`).
	//
	// Default (empty or `null`) value results in using the default context from K8s's config file.
	Contexts []string `yaml:"contexts,omitempty" json:"contexts" jsonschema:"minLength=1"`

	// The best effort maximum number of Go routines to use.
	// Lower this number to reduce memory usage.
	Concurrency int `yaml:"concurrency,omitempty" json:"concurrency" jsonschema:"minimum=1,default=50000"`

	// Custom resources to collect from the cluster.
	// Specify resources using GroupVersionKind format: "group/version/kind"
	// Example: "cert-manager.io/v1/Certificate"
	CustomResources []CustomResourceSpec `yaml:"custom_resources,omitempty" json:"custom_resources"`
}

// CustomResourceSpec defines a custom resource to collect.
type CustomResourceSpec struct {
	// GVK in format "group/version/kind" (e.g., "cert-manager.io/v1/Certificate")
	GVK string `yaml:"gvk" json:"gvk" jsonschema:"required,pattern=^[a-z0-9.-]+/[a-z0-9]+/[A-Z][a-zA-Z0-9]*$"`

	// Optional: Namespace filter. If empty, collects from all namespaces.
	Namespaces []string `yaml:"namespaces,omitempty" json:"namespaces"`
}

func (s *Spec) SetDefaults() {
	if s.Concurrency <= 0 {
		const defaultConcurrency = 50000
		s.Concurrency = defaultConcurrency
	}
}

// Validate validates the spec configuration.
func (s *Spec) Validate() error {
	var errs []error

	for i, cr := range s.CustomResources {
		if err := validateGVK(cr.GVK); err != nil {
			errs = append(errs, fmt.Errorf("custom_resources[%d].gvk: %w", i, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation errors: %v", errs)
	}

	return nil
}

var gvkPattern = regexp.MustCompile(`^[a-z0-9.-]+/[a-z0-9]+/[A-Z][a-zA-Z0-9]*$`)

// validateGVK validates the GroupVersionKind format.
func validateGVK(gvk string) error {
	if gvk == "" {
		return fmt.Errorf("GVK cannot be empty")
	}

	parts := strings.Split(gvk, "/")
	if len(parts) != 3 {
		return fmt.Errorf("invalid GVK format %q: must be 'group/version/kind'", gvk)
	}

	group, version, kind := parts[0], parts[1], parts[2]

	// Reject core resources (empty group or "core")
	if group == "" || group == "core" {
		return fmt.Errorf("invalid GVK %q: core Kubernetes resources are not supported via custom_resources (use dedicated tables instead)", gvk)
	}

	if version == "" {
		return fmt.Errorf("invalid GVK %q: version cannot be empty", gvk)
	}

	if kind == "" {
		return fmt.Errorf("invalid GVK %q: kind cannot be empty", gvk)
	}

	// Validate format with regex
	if !gvkPattern.MatchString(gvk) {
		return fmt.Errorf("invalid GVK format %q: group must be lowercase alphanumeric with dots/dashes, version lowercase alphanumeric, kind must start with uppercase", gvk)
	}

	return nil
}

//go:embed schema.json
var JSONSchema string
