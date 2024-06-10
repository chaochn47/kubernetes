/*
Copyright 2016 The Kubernetes Authors.

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

// Note: the example only works with the code within the same release/branch.
package main

import (
	"context"
	"flag"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"
	//
	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"
	//
	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

func main() {
	klog.InitFlags(nil)
	flag.Set("v", "2")
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	var useProto *bool
	useProto = flag.Bool("use-proto", false, "use protobuf as content type or not")

	flag.Parse()
	defer klog.Flush()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if *useProto {
		config.ContentType = "application/vnd.kubernetes.protobuf"
	}

	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	//listOption := metav1.ListOptions{
	//	FieldSelector:   "status.podIP=10.0.141.200,status.phase=Running",
	//	ResourceVersion: "0",
	//}
	startTime := time.Now()
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		Limit:           500,
		ResourceVersion: "0",
	})
	klog.Infof("took %s, pods resourceVersion %s, %d pods\n", time.Since(startTime).String(), pods.GetResourceVersion(), len(pods.Items))
	if err != nil {
		panic(err.Error())
	}
	//for i, po := range pods.Items {
	//	fmt.Printf("%dth pod name is %s, in namespae %s \n", i+1, po.Name, po.Namespace)
	//}
}
