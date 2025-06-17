//// Copyright 2017 Oracle and/or its affiliates. All rights reserved.
////
//// Licensed under the Apache License, Version 2.0 (the "License");
//// you may not use this file except in compliance with the License.
//// You may obtain a copy of the License at
////
////     http://www.apache.org/licenses/LICENSE-2.0
////
//// Unless required by applicable law or agreed to in writing, software
//// distributed under the License is distributed on an "AS IS" BASIS,
//// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//// See the License for the specific language governing permissions and
//// limitations under the License.
//
//package tags
//
//import (
//	"context"
//	"encoding/json"
//	"fmt"
//	cloudprovider "k8s.io/cloud-provider"
//
//	//utils "github.com/oracle/oci-cloud-controller-manager/pkg/cloudprovider/providers/oci"
//	ociconfig "github.com/oracle/oci-cloud-controller-manager/pkg/cloudprovider/providers/oci/config"
//	"github.com/oracle/oci-cloud-controller-manager/pkg/oci/client"
//	"github.com/oracle/oci-go-sdk/v65/core"
//	"go.uber.org/zap"
//	"gopkg.in/yaml.v2"
//	v1 "k8s.io/api/core/v1"
//	"k8s.io/apimachinery/pkg/labels"
//	"k8s.io/client-go/informers"
//	"k8s.io/client-go/kubernetes"
//	"k8s.io/client-go/tools/cache"
//	"log"
//	"os"
//	"strings"
//	"time"
//)
//
//const (
//	reconcileRetry = 2
//)
//
//func Run(ctx context.Context, clientset *kubernetes.Clientset, logger *zap.SugaredLogger, kubeconfig string) error {
//
//	tagsFile := ""
//	logger.Info("[INFO]:  Starting unmarshall tags")
//	expectedTags, err := LoadTagsFromFile(tagsFile)
//	if err != nil {
//		return fmt.Errorf("failed to load tags from file: %w", err)
//	}
//
//	factory := informers.NewSharedInformerFactory(clientset, 0)
//	nodeInformer := factory.Core().V1().Nodes().Informer()
//	nodeLister := factory.Core().V1().Nodes().Lister()
//
//	// Load OCI Auth config (only Auth block)
//	logger.Info("[INFO]:  Starting to build auth config")
//	ociCfg, err := LoadOCIAuthConfig(kubeconfig)
//	if err != nil {
//		return fmt.Errorf("failed to load OCI config: %w", err)
//	}
//	if err = ociCfg.Validate(); err != nil {
//		return err
//	}
//
//	// Build the SDK
//	logger.Info("[INFO]:  Starting to build sdk")
//	oClient, err := ociconfig.NewConfigurationProvider(ociCfg)
//	if err != nil {
//		return fmt.Errorf("failed to create OCI compute client: %w", err)
//	}
//	rateLimiter := client.NewRateLimiter(logger, ociCfg.RateLimiter)
//	ociClient, err := client.New(logger, oClient, &rateLimiter, ociCfg)
//	if err != nil {
//		return err
//	}
//
//	logger.Info("[INFO]:  Starting nodeInformer")
//	go nodeInformer.Run(ctx.Done())
//	if !cache.WaitForCacheSync(ctx.Done(), nodeInformer.HasSynced) {
//		return fmt.Errorf("failed to sync node informer cache")
//	}
//
//	ticker := time.NewTicker(reconcileRetry * time.Minute)
//	defer ticker.Stop()
//
//	for {
//		select {
//		case <-ctx.Done():
//			log.Println("[INFO] Controller stopped")
//			return nil
//		case <-ticker.C:
//			nodes, err := nodeLister.List(labels.Everything())
//			if err != nil {
//				log.Printf("[ERROR] Failed to list nodes: %v", err)
//				continue
//			}
//			logger.Info("[INFO]:  list all nodes returned from nodeLister", nodes)
//			for _, node := range nodes {
//				logger.Info("[INFO]:  processing node: ", node)
//				ReconcileNode(ctx, ociClient, node, expectedTags, logger)
//			}
//		}
//	}
//}
//
//// ReconcileNode  retrieves instance details, merges required tags with existing defined tags,
//// and calls the UpdateInstance API to apply the changes.
//func ReconcileNode(ctx context.Context, oClint client.Interface, node *v1.Node, expectedTags map[string]map[string]interface{}, logger *zap.SugaredLogger) {
//
//	logger.Info("[INFO]:  Getting instanceOcid for node: ", node)
//	instanceOCID, err := getInstanceIDFromNode(node, logger)
//	if err != nil {
//		logger.Error("[ERROR] Failed to instanceOCID for node %s: %v", node.Name, err)
//		return
//	}
//
//	logger.Info("[INFO]: instanceOcid: for node: ", instanceOCID, node.Name)
//	instance, err := oClint.Compute().GetInstance(ctx, instanceOCID)
//	if err != nil {
//		logger.Errorf("[ERROR] Failed to get instance for node %s: %v", node.Name, err)
//		return
//	}
//
//	logger.Infof("[INFO]: Existing tags on node: %v. Expected tags on node:  %v", instance.DefinedTags, expectedTags)
//
//	tags, _ := MergeTags(instance, expectedTags)
//
//	_, err = oClint.Compute().UpdateInstance(ctx, core.UpdateInstanceRequest{
//		InstanceId: &instanceOCID,
//		UpdateInstanceDetails: core.UpdateInstanceDetails{
//			DefinedTags: tags,
//		},
//	})
//	if err != nil {
//		log.Printf("[ERROR] Failed to update tags for node %s: %v", node.Name, err)
//	} else {
//		log.Printf("[INFO] Successfully updated tags for node %s", node.Name)
//	}
//}
//
//// getInstanceIDFromNode - Retrieves  the instanceOcid from the Node
//func getInstanceIDFromNode(node *v1.Node, logger *zap.SugaredLogger) (string, error) {
//	if node == nil {
//		return "", fmt.Errorf("node is nil")
//	}
//	logger.Info("Node providerId", node.Name)
//	providerID := node.Spec.ProviderID
//	if providerID == "" {
//		return "", fmt.Errorf("providerID is empty for node %s", node.Name)
//	}
//	if !strings.HasPrefix(providerID, "oci://") {
//		return "", fmt.Errorf("providerID %q for node %s is not prefixed with oci://", providerID, node.Name)
//	}
//	return strings.TrimPrefix(providerID, "oci://"), nil
//}
//
//func LoadTagsFromFile(path string) (map[string]map[string]interface{}, error) {
//	file, err := os.ReadFile(path)
//	if err != nil {
//		return nil, err
//	}
//	var tags map[string]map[string]interface{}
//	if err := json.Unmarshal(file, &tags); err != nil {
//		return nil, err
//	}
//	return tags, nil
//}
//
//// LoadOCIAuthConfig
//func LoadOCIAuthConfig(path string) (*ociconfig.Config, error) {
//	data, err := os.ReadFile(path)
//	if err != nil {
//		return nil, err
//	}
//	var cfg ociconfig.Config
//	if err := yaml.Unmarshal(data, &cfg); err != nil {
//		return nil, err
//	}
//	return &cfg, nil
//}
//
//// MergeTags - Retrieve all defined tags currently set on the instance,
//// and merge them with the required tags that should be present.
//func MergeTags(instance *core.Instance, expectedTags map[string]map[string]interface{}) (map[string]map[string]interface{}, error) {
//	d := instance.DefinedTags
//
//	for namespace, tags := range expectedTags {
//		// Ensure the namespace exists
//		if d[namespace] == nil {
//			d[namespace] = map[string]interface{}{}
//		}
//		// Add or update keys within the namespace
//		for k, v := range tags {
//			d[namespace][k] = v
//		}
//	}
//	return d, nil
//}
