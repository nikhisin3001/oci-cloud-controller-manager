// Copyright 2018 Oracle and/or its affiliates. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tags

import (
	"context"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var (
	instances = map[string]*core.Instance{
		"instance1": {
			CompartmentId: common.String("compartment1"),
			Id:            common.String("ocid1.instance1"),
			Shape:         common.String("VM.Standard1.2"),
			DisplayName:   common.String("instance1"),
		},
		"openshift-instance-ipv4": {
			Id:            common.String("ocid1.openshift-instance-ipv4"),
			CompartmentId: common.String("default"),
			DefinedTags: map[string]map[string]interface{}{
				"openshift-namespace": {
					"role": "compute",
				},
			},
		},
	}
)

// MockComputeClient mocks Compute client implementation
type MockComputeClient struct{}

func (MockComputeClient) GetInstance(ctx context.Context, id string) (*core.Instance, error) {
	if instance, ok := instances[id]; ok {
		return instance, nil
	}
	return &core.Instance{
		AvailabilityDomain: common.String("NWuj:PHX-AD-1"),
		CompartmentId:      common.String("default"),
		Id:                 &id,
		Region:             common.String("PHX"),
		Shape:              common.String("VM.Standard1.2"),
	}, nil
}

func TestUnMarShallTags(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "tags.json")
	jsonContent := `{
		"openshift-tags": {
			"openshift-resource": "openshift-resource-infra",
			"cluster-id": "ocid1.cluster.oc1..abc"
		},
		"team-tags": {
			"owner": "platform-team"
		}
	}`
	if err := os.WriteFile(tmpFile, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tags, err := LoadTagsFromFile(tmpFile)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if tags["openshift-tags"]["openshift-resource"] != "openshift-resource-infra" {
		t.Errorf("unexpected value for openshift-resource tag")
	}
	if tags["team-tags"]["owner"] != "platform-team" {
		t.Errorf("unexpected value for team owner tag")
	}

}

func TestLoadTagsFromFile_InvalidJSON(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "invalid.json")
	invalidJSON := `this is not json`
	if err := os.WriteFile(tmpFile, []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := LoadTagsFromFile(tmpFile)
	if err == nil {
		t.Errorf("expected error for invalid JSON, got nil")
	}
}

func TestLoadTagsFromFile_FileNotFound(t *testing.T) {
	_, err := LoadTagsFromFile("nonexistent.json")
	if err == nil {
		t.Errorf("expected error for missing file, got nil")
	}
}

func TestMergeTags(t *testing.T) {
	instance := &core.Instance{
		DefinedTags: map[string]map[string]interface{}{
			"openshift-tags": {
				"existing": "keep-me",
			},
		},
	}

	expected := map[string]map[string]interface{}{
		"openshift-tags": {
			"openshift-resource": "openshift-resource-infra",
		},
		"team-tags": {
			"owner": "platform-team",
		},
	}

	merged, err := MergeTags(instance, expected)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]map[string]interface{}{
		"openshift-tags": {
			"existing":           "keep-me",
			"openshift-resource": "openshift-resource-infra",
		},
		"team-tags": {
			"owner": "platform-team",
		},
	}

	if !reflect.DeepEqual(merged, want) {
		t.Errorf("merged tags did not match expected.\nGot:  %#v\nWant: %#v", merged, want)
	}
}

func TestLoadOCIAuthConfig(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "cloud-provider.yaml")
	jsonContent := `useInstancePrincipals: true
    compartment: $COMPARTMENT_ID
    vcn: $OCP_VCN_ID
    loadBalancer:
      subnet1: $OCP_SUBNET_ID
      securityListManagementMode: Frontend
      securityLists:
        $OCP_SUBNET_ID: $OPC_SEC_LIST_ID
    rateLimiter:
      rateLimitQPSRead: 20.0
      rateLimitBucketRead: 5
      rateLimitQPSWrite: 20.0
      rateLimitBucketWrite: 5`
	if err := os.WriteFile(tmpFile, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	c, err := LoadOCIAuthConfig(tmpFile)

}
