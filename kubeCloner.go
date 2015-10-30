/*
Copyright 2014 The Kubernetes Authors All rights reserved.

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

// kube2consul is a bridge between Kubernetes and Consul.  It watches the
// Kubernetes master for changes in Services and creates new DNS records on the
// consul agent.

package main // import "github.com/jmccarty3/kubeClonner"

import (
	"flag"
	"fmt"
//	"strings"
//	"net/url"
//	"os"
//	"time"
//	"reflect"
//	"strconv"

	kapi "k8s.io/kubernetes/pkg/api"
	kclient "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/labels"
//	"k8s.io/kubernetes/pkg/util"
	"github.com/golang/glog"
)

var (
	argSourceUrl  = flag.String("source", "", "Source URL")
	argSinkUrl    = flag.String("sink", "", "Sink URL")
	argConOnEror  = flag.Bool("continue_on_error", false, "Continuie Cloning on error. Useful for cloning the system namespace")
	argRollback   = flag.Bool("rollback", false, "Rollback all changes to sink on error")
	argNamespace  = flag.String("namespace", "", "Namespace to clone from. If blank all namespaces.")
)

type cloner struct {
	// Source client.
	source *kclient.Client

	// Sink client.
	sink *kclient.Client

	//Nodes Name / valid
	rc []kapi.ObjectMeta

	//All Services.
	svc []kapi.ObjectMeta
}

func HandleError(clone *cloner, msg string)(){
	glog.Error(msg)

	if *argConOnEror {
		return
	}	else if *argRollback {
		glog.Info("Performing rollback")

		for _,rc := range clone.rc{
			clone.sink.ReplicationControllers(rc.Namespace).Delete(rc.Name)
			glog.Info("Rolled back RC: ", rc.Name)
		}
		for _,svc := range clone.svc{
			clone.sink.Services(svc.Namespace).Delete(svc.Name)
			glog.Info("Rolled back Service: ", svc.Name)
		}
	}

	glog.Fatal("Exiting")
}

func MakeObjectMeta(meta *kapi.ObjectMeta)(kapi.ObjectMeta){
	newMeta := kapi.ObjectMeta{
		Name: meta.Name,
		Namespace: meta.Namespace,
		DeletionGracePeriodSeconds: meta.DeletionGracePeriodSeconds,
		Labels: meta.Labels,
		Annotations: meta.Annotations,
	}

	return newMeta
}

func CloneRC(clone *cloner, rc *kapi.ReplicationController) (){
	newRC := kapi.ReplicationController{
		ObjectMeta: MakeObjectMeta(&rc.ObjectMeta),
		Spec: rc.Spec,
	}

	if _,err := clone.sink.ReplicationControllers(rc.ObjectMeta.Namespace).Create(&newRC); err != nil{
		HandleError(clone, fmt.Sprint("Failure to create RC. ", err))
	}

	clone.rc = append(clone.rc, rc.ObjectMeta)
}

func CloneService(clone *cloner, svc *kapi.Service) () {
	newService := kapi.Service{
		ObjectMeta: MakeObjectMeta(&svc.ObjectMeta),
		Spec: svc.Spec,
	}

	var err error

	if _,err = clone.sink.Services(svc.ObjectMeta.Namespace).Create(&newService); err != nil {
		HandleError(clone, fmt.Sprintf("Failure to create service: %s Error: %s", newService, err))
	}

	clone.svc = append(clone.svc, svc.ObjectMeta)
}

func CloneNamespace(clone *cloner, ns string)(){
	glog.Info("Cloning Namespace: ", ns)
	svcList, err := clone.source.Services(ns).List(labels.Everything())

	if err != nil{
		glog.Fatal("Could not get all services. ", err)
	}
	for _,svc := range svcList.Items{
		if svc.ObjectMeta.Name != "kubernetes"{
			glog.Info("Cloning Service: ", svc.ObjectMeta.Name)
			CloneService(clone,&svc)
		}
	}

	rcList, err := clone.source.ReplicationControllers(ns).List(labels.Everything())

	for _,rc := range rcList.Items{
		glog.Info("Cloning RC: ", rc.ObjectMeta.Name)
		CloneRC(clone,&rc)
	}
}

func main() {
	flag.Parse()
	var err error
	var clone cloner

	sourceConfig := &kclient.Config{
		Host: *argSourceUrl,
		Version: "v1",
	}

	sinkConfig := &kclient.Config{
		Host: *argSinkUrl,
		Version: "v1",
	}

	clone.source,err = kclient.New(sourceConfig)

	if err != nil{
		return
	}
	clone.sink,err = kclient.New(sinkConfig)

	if err != nil {
		return
	}

	if *argNamespace != ""{
		CloneNamespace(&clone,*argNamespace)
	} else {
		ns,err := clone.source.Namespaces().List(labels.Everything(), fields.Everything())

		if err != nil {
			glog.Fatal("Unable to list all namespaces")
		}

		for _,item := range ns.Items {
			CloneNamespace(&clone, item.ObjectMeta.Name)
		}

		//Doing system directly since it is not returned in the enum
		CloneNamespace(&clone, "kube-system")
	}

	glog.Info("Clonned")
}
