package spec

import (
	"testing"

	"github.com/cloudquery/codegen/jsonschema"
)

func TestSpecJSONSchema(t *testing.T) {
	jsonschema.TestJSONSchema(t, JSONSchema, []jsonschema.TestCase{
		{
			Name: "empty",
			Spec: `{}`,
		},
		{
			Name: "empty contexts",
			Spec: `{"contexts":[]}`,
		},
		{
			Name: "null contexts",
			Spec: `{"contexts":null}`,
		},
		{
			Name: "bad contexts",
			Err:  true,
			Spec: `{"contexts":123}`,
		},
		{
			Name: "empty contexts entry",
			Err:  true,
			Spec: `{"contexts":[""]}`,
		},
		{
			Name: "null contexts entry",
			Err:  true,
			Spec: `{"contexts":[null]}`,
		},
		{
			Name: "bad contexts entry",
			Err:  true,
			Spec: `{"contexts":[123]}`,
		},
		{
			Name: "proper contexts entry",
			Spec: `{"contexts":["some-ctx"]}`,
		},
		{
			Name: "zero concurrency",
			Err:  true,
			Spec: `{"concurrency":0}`,
		},
		{
			Name: "null concurrency",
			Err:  true,
			Spec: `{"concurrency":null}`,
		},
		{
			Name: "bad concurrency",
			Err:  true,
			Spec: `{"concurrency":3.5}`,
		},
		{
			Name: "proper concurrency",
			Spec: `{"concurrency":5}`,
		},
	})
}

func TestValidateGVK(t *testing.T) {
	tests := []struct {
		name    string
		gvk     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid GVK",
			gvk:     "cert-manager.io/v1/Certificate",
			wantErr: false,
		},
		{
			name:    "valid GVK with alpha version",
			gvk:     "argoproj.io/v1alpha1/Application",
			wantErr: false,
		},
		{
			name:    "valid GVK with beta version",
			gvk:     "networking.k8s.io/v1beta1/Ingress",
			wantErr: false,
		},
		{
			name:    "empty GVK",
			gvk:     "",
			wantErr: true,
			errMsg:  "GVK cannot be empty",
		},
		{
			name:    "missing version",
			gvk:     "cert-manager.io/Certificate",
			wantErr: true,
			errMsg:  "must be 'group/version/kind'",
		},
		{
			name:    "missing kind",
			gvk:     "cert-manager.io/v1",
			wantErr: true,
			errMsg:  "must be 'group/version/kind'",
		},
		{
			name:    "lowercase kind",
			gvk:     "cert-manager.io/v1/certificate",
			wantErr: true,
			errMsg:  "kind must start with uppercase",
		},
		{
			name:    "core resource with empty group",
			gvk:     "/v1/Pod",
			wantErr: true,
			errMsg:  "core Kubernetes resources are not supported",
		},
		{
			name:    "core resource",
			gvk:     "core/v1/Pod",
			wantErr: true,
			errMsg:  "core Kubernetes resources are not supported",
		},
		{
			name:    "valid with dashes in group",
			gvk:     "cert-manager.io/v1/Certificate",
			wantErr: false,
		},
		{
			name:    "valid with multiple dots in group",
			gvk:     "networking.k8s.io/v1/Ingress",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGVK(tt.gvk)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateGVK() expected error but got none")
				} else if tt.errMsg != "" && !stringContains(err.Error(), tt.errMsg) {
					t.Errorf("validateGVK() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateGVK() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestSpec_Validate(t *testing.T) {
	tests := []struct {
		name    string
		spec    *Spec
		wantErr bool
	}{
		{
			name: "valid spec with single custom resource",
			spec: &Spec{
				CustomResources: []CustomResourceSpec{
					{GVK: "cert-manager.io/v1/Certificate"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid spec with multiple custom resources",
			spec: &Spec{
				CustomResources: []CustomResourceSpec{
					{GVK: "cert-manager.io/v1/Certificate"},
					{GVK: "argoproj.io/v1alpha1/Application"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid spec with namespace filter",
			spec: &Spec{
				CustomResources: []CustomResourceSpec{
					{
						GVK:        "cert-manager.io/v1/Certificate",
						Namespaces: []string{"default", "production"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid spec with empty custom resources",
			spec: &Spec{
				CustomResources: []CustomResourceSpec{},
			},
			wantErr: false,
		},
		{
			name: "invalid spec with bad GVK",
			spec: &Spec{
				CustomResources: []CustomResourceSpec{
					{GVK: "invalid"},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec with core resource",
			spec: &Spec{
				CustomResources: []CustomResourceSpec{
					{GVK: "core/v1/Pod"},
				},
			},
			wantErr: true,
		},
		{
			name: "spec with mix of valid and invalid",
			spec: &Spec{
				CustomResources: []CustomResourceSpec{
					{GVK: "cert-manager.io/v1/Certificate"},
					{GVK: "invalid-gvk"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Spec.Validate() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Spec.Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func stringContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && stringContainsHelper(s, substr))
}

func stringContainsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
