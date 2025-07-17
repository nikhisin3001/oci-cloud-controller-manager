package oci

import (
	"github.com/oracle/oci-cloud-controller-manager/pkg/cloudprovider/providers/oci/config"
	"github.com/oracle/oci-go-sdk/v65/core"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"strings"
	"testing"
)

func newTestLogger(t *testing.T) *zap.SugaredLogger {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	return logger.Sugar()
}

func TestCheckForOpenShiftClusterType_NoOpenShiftNamespace(t *testing.T) {
	tc := &TaggingController{}
	instance := &core.Instance{
		DefinedTags: map[string]map[string]interface{}{
			"foo": {"bar": "baz"},
		},
	}
	initial := &config.TagConfig{
		FreeformTags: map[string]string{"foo": "bar"},
		DefinedTags: map[string]map[string]interface{}{
			"foo": {"bar": "baz"},
		},
	}
	result := tc.checkForOpenShiftClusterType(instance, initial)
	if !reflect.DeepEqual(initial, result) {
		t.Error("Expected tag config unchanged when no OpenShift namespace")
	}
}

func TestCheckForOpenShiftClusterType_MatchAddsTag(t *testing.T) {
	tc := &TaggingController{}
	instance := &core.Instance{
		DefinedTags: map[string]map[string]interface{}{
			"openshift-tags": {"preexisting": "tag"},
		},
	}
	initial := &config.TagConfig{
		DefinedTags: map[string]map[string]interface{}{},
	}
	result := tc.checkForOpenShiftClusterType(instance, initial)
	if got, want := result.DefinedTags[openshiftTagNamespace][openshiftTagKey], openshiftTagValue; got != want {
		t.Errorf("Expected OpenShift tag [%s]=%s, got %v", openshiftTagKey, openshiftTagValue, got)
	}
}

func TestCheckForOpenShiftClusterType_MatchWithOtherTags(t *testing.T) {
	tc := &TaggingController{}
	instance := &core.Instance{
		DefinedTags: map[string]map[string]interface{}{
			"openshift-tags": {"role": "infra"},
			"other-ns":       {"foo": "bar"},
		},
	}
	initial := &config.TagConfig{
		DefinedTags: map[string]map[string]interface{}{
			"other-ns": {"foo": "bar"},
		},
	}
	result := tc.checkForOpenShiftClusterType(instance, initial)
	if got := result.DefinedTags["openshift-tags"][openshiftTagKey]; got != openshiftTagValue {
		t.Errorf("Expected OpenShift tag [%s]=%s, got %v", openshiftTagKey, openshiftTagValue, got)
	}
	if got := result.DefinedTags["other-ns"]["foo"]; got != "bar" {
		t.Errorf("Expected other tags to be copied, got %v", got)
	}
}

// --- Example MergeTags test ---

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

func TestCheckForOpenShiftClusterType(t *testing.T) {
	tc := &TaggingController{}

	tests := []struct {
		name     string
		instance *core.Instance
		input    *config.TagConfig
		want     *config.TagConfig
	}{
		{
			name: "No OpenShift Namespace",
			instance: &core.Instance{
				DefinedTags: map[string]map[string]interface{}{
					"some-ns": {"foo": "bar"},
				},
			},
			input: &config.TagConfig{
				FreeformTags: map[string]string{"a": "b"},
				DefinedTags: map[string]map[string]interface{}{
					"some-ns": {"foo": "bar"},
				},
			},
			want: &config.TagConfig{
				FreeformTags: map[string]string{"a": "b"},
				DefinedTags: map[string]map[string]interface{}{
					"some-ns": {"foo": "bar"},
				},
			},
		},
		{
			name: "OpenShift Namespace Present",
			instance: &core.Instance{
				DefinedTags: map[string]map[string]interface{}{
					"openshift-tags": {"x": "y"},
				},
			},
			input: &config.TagConfig{
				FreeformTags: map[string]string{"c": "d"},
				DefinedTags:  map[string]map[string]interface{}{},
			},
			want: &config.TagConfig{
				FreeformTags: map[string]string{"c": "d"},
				DefinedTags: map[string]map[string]interface{}{
					"openshift-tags": {
						"openshift-resource": "openshift-resource-infra",
					},
				},
			},
		},
		{
			name: "OpenShift Namespace, Already Exists in Input",
			instance: &core.Instance{
				DefinedTags: map[string]map[string]interface{}{
					"openshift-tags": {"something": "else"},
				},
			},
			input: &config.TagConfig{
				FreeformTags: map[string]string{"k": "v"},
				DefinedTags: map[string]map[string]interface{}{
					"openshift-tags": {"preexist": "tag"},
				},
			},
			want: &config.TagConfig{
				FreeformTags: map[string]string{"k": "v"},
				DefinedTags: map[string]map[string]interface{}{
					"openshift-tags": {
						"preexist":           "tag",
						"openshift-resource": "openshift-resource-infra",
					},
				},
			},
		},
		{
			name: "Nil DefinedTags in Input",
			instance: &core.Instance{
				DefinedTags: map[string]map[string]interface{}{
					"openshift-tags": {},
				},
			},
			input: &config.TagConfig{
				FreeformTags: map[string]string{"f": "g"},
				DefinedTags:  nil,
			},
			want: &config.TagConfig{
				FreeformTags: map[string]string{"f": "g"},
				DefinedTags: map[string]map[string]interface{}{
					"openshift-tags": {"openshift-resource": "openshift-resource-infra"},
				},
			},
		},
		{
			name: "Nil Instance DefinedTags",
			instance: &core.Instance{
				DefinedTags: nil,
			},
			input: &config.TagConfig{
				FreeformTags: map[string]string{"a": "b"},
				DefinedTags:  map[string]map[string]interface{}{},
			},
			want: &config.TagConfig{
				FreeformTags: map[string]string{"a": "b"},
				DefinedTags:  map[string]map[string]interface{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.checkForOpenShiftClusterType(tt.instance, tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Expected %+v, got %+v", tt.want, got)
			}
		})
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
			wantID:    "",
			wantError: `providerID "aws://i-xxxxx" for node node3 is not prefixed with oci://`,
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
