// Copyright 2017 Oracle and/or its affiliates. All rights reserved.
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

package oci

import (
	"context"
	"fmt"
	"github.com/oracle/oci-cloud-controller-manager/pkg/cloudprovider/providers/oci/config"
	"github.com/oracle/oci-cloud-controller-manager/pkg/util"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"log"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/oracle/oci-cloud-controller-manager/pkg/oci/client"
	"github.com/oracle/oci-go-sdk/v65/core"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

const (
	reconcileRetry        = 2
	openshiftTagNamespace = "openshift-tags"
	openshiftTagKey       = "openshift-resource"
	openshiftTagValue     = "openshift-resource-infra"
)

type TaggingController struct {
	nodeInformer  coreinformers.NodeInformer
	logger        *zap.SugaredLogger
	kubeClient    clientset.Interface
	recorder      record.EventRecorder
	cloud         *CloudProvider
	queue         workqueue.RateLimitingInterface
	instanceCache cache.Store
	ociClient     client.Interface
}

// NewTaggingController creates a TaggingController object
func NewTaggingController(
	nodeInformer coreinformers.NodeInformer,
	kubeClient kubernetes.Interface,
	cloud *CloudProvider,
	logger *zap.SugaredLogger,
	instanceCache cache.Store,
	ociClient client.Interface) *TaggingController {

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "tagging-controller"})
	eventBroadcaster.StartLogging(klog.Infof)
	if kubeClient != nil {
		logger.Info("Sending events to api server.")
		eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	} else {
		logger.Info("No api server defined - no events will be sent to API server.")
	}

	tc := &TaggingController{
		nodeInformer:  nodeInformer,
		kubeClient:    kubeClient,
		logger:        logger,
		recorder:      recorder,
		cloud:         cloud,
		queue:         workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		instanceCache: instanceCache,
		ociClient:     ociClient,
	}

	// Use shared informer to listen to add nodes
	tc.nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node := obj.(*v1.Node)
			tc.queue.Add(node.Name)
		},
		UpdateFunc: func(_, newObj interface{}) {
			node := newObj.(*v1.Node)
			tc.queue.Add(node.Name)
		},
	})

	return tc

}

func (tc *TaggingController) Run(stopCh <-chan struct{}) {
	defer tc.queue.ShutDown()
	tc.logger.Info("Starting tagging controller")
	wait.Until(func() {
		if err := tc.runWorker(stopCh); err != nil {
			klog.Errorf("runWorker error: %v", err)
		}
	}, time.Second, stopCh)

}

func (tc *TaggingController) runWorker(stopCh <-chan struct{}) error {

	//ctx := context.Background()

	tc.logger.Info("[INFO]:  Starting unmarshall tags")

	nodeLister := tc.nodeInformer.Lister()

	ticker := time.NewTicker(reconcileRetry * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			log.Println("[INFO] Controller stopped")
			return nil
		case <-ticker.C:
			nodes, err := nodeLister.List(labels.Everything())
			if err != nil {
				log.Printf("[ERROR] Failed to list nodes: %v", err)
				continue
			}
			tc.logger.Info("[INFO]:  list all nodes returned from nodeLister", nodes)
			for _, node := range nodes {
				tc.logger.Info("[INFO]:  processing node: ", node)
				tc.ReconcileNode(context.Background(), node)
			}
		}
	}
}

// ReconcileNode  retrieves instance details, merges required tags with existing defined tags,
// and calls the UpdateInstance API to apply the changes.
func (tc *TaggingController) ReconcileNode(ctx context.Context, node *v1.Node) {

	tc.logger.Info("[INFO]:  Getting instanceOcid for node: ", node)
	instanceOCID, err := getInstanceIDFromNode(node, tc.logger)
	if err != nil {
		tc.logger.Error("[ERROR] Failed to instanceOCID for node %s: %v", node.Name, err)
		return
	}

	tc.logger.Info("[INFO]: instanceOcid: for node: ", instanceOCID, node.Name)
	instance, err := tc.ociClient.Compute().GetInstance(ctx, instanceOCID)
	if err != nil {
		tc.logger.Errorf("[ERROR] Failed to get instance for node %s: %v", node.Name, err)
		return
	}

	tc.logger.Infof("[INFO]: Existing tags on node: %v. Expected tags on node:  ", instance.DefinedTags)

	var t *config.TagConfig
	if tc.cloud.config.Tags == nil || tc.cloud.config.Tags.Common == nil {
		tc.logger.Warnf("[WARN] Tag config is nil; using empty TagConfig for node %s", node.Name)
		t = &config.TagConfig{
			FreeformTags: map[string]string{},
			DefinedTags:  map[string]map[string]interface{}{},
		}
	} else {
		t = tc.cloud.config.Tags.Common
	}

	// If cluster is OpenShift, then make sure the required OpenShift
	// tags are applied
	t = tc.checkForOpenShiftClusterType(instance, t)

	tags := MergeTags(instance, t)

	_, err = tc.ociClient.Compute().UpdateInstance(ctx, core.UpdateInstanceRequest{
		InstanceId: &instanceOCID,
		UpdateInstanceDetails: core.UpdateInstanceDetails{
			DefinedTags:  tags.DefinedTags,
			FreeformTags: tags.FreeformTags,
		},
	})
	if err != nil {
		log.Printf("[ERROR] Failed to update tags for node %s: %v", node.Name, err)
	} else {
		log.Printf("[INFO] Successfully updated tags for node %s", node.Name)
	}
}

// getInstanceIDFromNode - Retrieves  the instanceOcid from the Node
func getInstanceIDFromNode(node *v1.Node, logger *zap.SugaredLogger) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}
	logger.Info("Node providerId", node.Name)
	providerID := node.Spec.ProviderID
	if providerID == "" {
		return "", fmt.Errorf("providerID is empty for node %s", node.Name)
	}
	if !strings.HasPrefix(providerID, "oci://") {
		return "", fmt.Errorf("providerID %q for node %s is not prefixed with oci://", providerID, node.Name)
	}
	return strings.TrimPrefix(providerID, "oci://"), nil
}

// MergeTags - Retrieve all defined tags currently set on the instance,
// and merge them with the required tags that should be present.
func MergeTags(instance *core.Instance, expectedTags *config.TagConfig) *config.TagConfig {

	srcTags := &config.TagConfig{
		FreeformTags: instance.FreeformTags,
		DefinedTags:  instance.DefinedTags,
	}
	return util.MergeTagConfig(srcTags, expectedTags)

}

func (tc *TaggingController) checkForOpenShiftClusterType(instance *core.Instance, t *config.TagConfig) *config.TagConfig {

	for namespace := range instance.DefinedTags {
		if strings.HasPrefix(namespace, OpenShiftTagNamespacePrefix) {
			// Copy the original TagConfig to avoid mutating input
			newTagConfig := &config.TagConfig{
				FreeformTags: t.FreeformTags,
				DefinedTags:  make(map[string]map[string]interface{}),
			}
			// Copy existing DefinedTags
			for ns, tags := range t.DefinedTags {
				newTagConfig.DefinedTags[ns] = make(map[string]interface{})
				for k, v := range tags {
					newTagConfig.DefinedTags[ns][k] = v
				}
			}
			// Add/overwrite the constant tag in the matching namespace
			if _, ok := newTagConfig.DefinedTags[openshiftTagNamespace]; !ok {
				newTagConfig.DefinedTags[openshiftTagNamespace] = make(map[string]interface{})
			}
			newTagConfig.DefinedTags[openshiftTagNamespace][openshiftTagKey] = openshiftTagValue
			return newTagConfig
		}
	}
	return t

}
