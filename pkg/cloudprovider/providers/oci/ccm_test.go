/*
Copyright 2025 Oracle and/or its affiliates. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package oci

import (
	"testing"

	"k8s.io/apimachinery/pkg/labels"
)

func TestParseOpenShiftLabelSelector_KeyOnly(t *testing.T) {
	selector, err := parseOpenShiftLabelSelector("node.openshift.io/os_id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	withKey := labels.Set{"node.openshift.io/os_id": "rhel"}
	if !selector.Matches(withKey) {
		t.Fatalf("expected selector to match when key exists regardless of value")
	}

	withKeyDifferentValue := labels.Set{"node.openshift.io/os_id": "fedora"}
	if !selector.Matches(withKeyDifferentValue) {
		t.Fatalf("expected selector to match when key exists with different value")
	}

	withoutKey := labels.Set{"some.other/label": "x"}
	if selector.Matches(withoutKey) {
		t.Fatalf("did not expect selector to match when key does not exist")
	}
}

func TestParseOpenShiftLabelSelector_KeyValue(t *testing.T) {
	selector, err := parseOpenShiftLabelSelector("node.openshift.io/os_id=rhel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	matching := labels.Set{"node.openshift.io/os_id": "rhel"}
	if !selector.Matches(matching) {
		t.Fatalf("expected selector to match when key=value exactly matches")
	}

	nonMatchingValue := labels.Set{"node.openshift.io/os_id": "fedora"}
	if selector.Matches(nonMatchingValue) {
		t.Fatalf("did not expect selector to match when value differs")
	}

	withoutKey := labels.Set{"some.other/label": "x"}
	if selector.Matches(withoutKey) {
		t.Fatalf("did not expect selector to match when key does not exist")
	}
}

func TestParseOpenShiftLabelSelector_Invalid(t *testing.T) {
	invalids := []string{
		"",
		"=",
		"node.openshift.io/os_id=",
		"=rhel",
	}

	for _, in := range invalids {
		if _, err := parseOpenShiftLabelSelector(in); err == nil {
			t.Fatalf("expected error for invalid input %q", in)
		}
	}
}
