package oci

import (
	"strings"
	"testing"

	"github.com/oracle/oci-cloud-controller-manager/pkg/cloudprovider/providers/oci/config"
	"github.com/oracle/oci-go-sdk/v65/core"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestLogger(t *testing.T) *zap.SugaredLogger {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	return logger.Sugar()
}

func TestHasRequiredDefinedTags(t *testing.T) {
	tc := &TaggingController{}
	tests := []struct {
		name     string
		instance *core.Instance
		required *config.TagConfig
		want     bool
	}{
		{
			name:     "no requirements",
			instance: &core.Instance{},
			required: nil,
			want:     true,
		},
		{
			name: "all defined tags present",
			instance: &core.Instance{
				DefinedTags: map[string]map[string]interface{}{
					"openshift-tags": {openshiftTagKey: openshiftTagValue},
				},
			},
			required: &config.TagConfig{
				DefinedTags: map[string]map[string]interface{}{
					"openshift-tags": {openshiftTagKey: openshiftTagValue},
				},
			},
			want: true,
		},
		{
			name: "missing defined namespace",
			instance: &core.Instance{
				DefinedTags: map[string]map[string]interface{}{},
			},
			required: &config.TagConfig{
				DefinedTags: map[string]map[string]interface{}{
					"openshift-tags": {openshiftTagKey: openshiftTagValue},
				},
			},
			want: false,
		},
		{
			name: "missing defined tag value",
			instance: &core.Instance{
				DefinedTags: map[string]map[string]interface{}{
					"openshift-tags": {"other": "value"},
				},
			},
			required: &config.TagConfig{
				DefinedTags: map[string]map[string]interface{}{
					"openshift-tags": {openshiftTagKey: openshiftTagValue},
				},
			},
			want: false,
		},
		{
			name: "no required defined tags",
			instance: &core.Instance{
				DefinedTags: map[string]map[string]interface{}{
					"foo": {"bar": "baz"},
				},
			},
			required: &config.TagConfig{DefinedTags: map[string]map[string]interface{}{}},
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tc.hasRequiredDefinedTags(tt.instance.DefinedTags, tt.required); got != tt.want {
				t.Fatalf("hasRequiredDefinedTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- MergeTags test ---

func TestMergeTags(t *testing.T) {
	instance := &core.Instance{
		FreeformTags: map[string]string{"foo": "bar"},
		DefinedTags: map[string]map[string]interface{}{
			"a": {"x": "y"},
		},
	}
	expected := &config.TagConfig{
		FreeformTags: map[string]string{"test": "123"},
		DefinedTags: map[string]map[string]interface{}{
			"b": {"cool": "tag"},
		},
	}
	result := MergeTags(instance, expected)
	if result.FreeformTags["foo"] != "bar" || result.FreeformTags["test"] != "123" {
		t.Errorf("Expected merged freeform tags, got %v", result.FreeformTags)
	}
	if result.DefinedTags["a"]["x"] != "y" || result.DefinedTags["b"]["cool"] != "tag" {
		t.Errorf("Expected merged defined tags, got %v", result.DefinedTags)
	}
}

func TestGetInstanceIDFromNode(t *testing.T) {
	logger := newTestLogger(t)
	tests := []struct {
		name      string
		node      *v1.Node
		wantID    string
		wantError string
	}{
		{
			name: "valid oci provider",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mynode",
				},
				Spec: v1.NodeSpec{
					ProviderID: "oci://ocid1.instance.oc1..aaaaaaaaxxx",
				},
			},
			wantID:    "ocid1.instance.oc1..aaaaaaaaxxx",
			wantError: "",
		},
		{
			name:      "nil node",
			node:      nil,
			wantID:    "",
			wantError: "node is nil",
		},
		{
			name: "empty provider id",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node2"},
				Spec:       v1.NodeSpec{ProviderID: ""},
			},
			wantID:    "",
			wantError: "providerID is empty for node node2",
		},
		{
			name: "wrong provider prefix",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node3"},
				Spec:       v1.NodeSpec{ProviderID: "aws://i-xxxxx"},
			},
			wantID:    "aws://i-xxxxx",
			wantError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, err := getInstanceIDFromNode(tt.node, logger)
			if gotID != tt.wantID {
				t.Errorf("got ID %q, want %q", gotID, tt.wantID)
			}
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Errorf("expected error %q, got %v", tt.wantError, err)
				}
			} else if err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}
