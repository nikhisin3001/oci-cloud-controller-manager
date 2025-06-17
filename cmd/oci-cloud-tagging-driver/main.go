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

package main

import (
	"context"
	"flag"
	"github.com/oracle/oci-cloud-controller-manager/pkg/logging"
	"github.com/oracle/oci-cloud-controller-manager/pkg/tags"
	"github.com/oracle/oci-cloud-controller-manager/pkg/util/signals"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"time"
)

var tagsFile string
var kubeconfig string

func main() {
	log := logging.Logger()
	logger := log.Sugar()
	logger.Info("[INFO]:  Starting Tagging Controller")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stopCh := signals.SetupSignalHandler()

	flag.StringVar(&tagsFile, "tags", "", "Tags to apply to OCI resources in the tagging controller, in a form of ConfigMap")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "cluster kubeconfig")
	flag.Parse()
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	clientset, err := kubernetes.NewForConfig(config)
	err = wait.PollUntil(15*time.Second, func() (done bool, err error) {
		_, err = clientset.Discovery().ServerVersion()
		if err != nil {
			logger.With(zap.Error(err)).Info("failed to get kube-apiserver version, will retry again")
			return false, nil
		}
		return true, nil
	}, stopCh)
	if err != nil {
		logger.With(zap.Error(err)).Errorf("failed to get kube-apiserver version")
		return
	}

	if err := tags.Run(ctx, tagsFile, clientset, logger, kubeconfig); err != nil {
		logger.Error("[FATAL] Controller execution failed: %v", err)
	}
}
