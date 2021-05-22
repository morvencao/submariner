/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the Submariner project.

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
package ipam

import (
	"fmt"
	"reflect"

	"github.com/submariner-io/admiral/pkg/federate"
	"github.com/submariner-io/admiral/pkg/log"
	"github.com/submariner-io/admiral/pkg/syncer"
	"github.com/submariner-io/admiral/pkg/util"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

func NewGlobalEgressIPController(config syncer.ResourceSyncerConfig, pool *IPPool) (ResourceController, error) {
	var err error

	klog.Info("Creating GlobalEgressIP controller")

	// TODO - get list of existing GlobalEgressIPs and prime the IPPool cache.

	controller := &globalEgressIPController{pool: pool}
	controller.resourceSyncer, err = syncer.NewResourceSyncer(&syncer.ResourceSyncerConfig{
		Name:                "GlobalEgressIP syncer",
		ResourceType:        &submarinerv1.GlobalEgressIP{},
		SourceClient:        config.SourceClient,
		SourceNamespace:     corev1.NamespaceAll,
		RestMapper:          config.RestMapper,
		Federator:           federate.NewUpdateStatusFederator(config.SourceClient, config.RestMapper, corev1.NamespaceAll),
		Scheme:              config.Scheme,
		Transform:           controller.process,
		ResourcesEquivalent: areSpecsEquivalent,
	})

	if err != nil {
		return nil, err
	}

	return controller, nil
}

func (c *globalEgressIPController) Start(stopCh <-chan struct{}) error {
	klog.Info("Starting GlobalEgressIP controller")

	return c.resourceSyncer.Start(stopCh)
}

func (c *globalEgressIPController) process(from runtime.Object, numRequeues int, op syncer.Operation) (runtime.Object, bool) {
	globalEgressIP := from.(*submarinerv1.GlobalEgressIP)

	klog.Infof("Processing %sd %#v", op, globalEgressIP)

	switch op {
	case syncer.Create:
		prevStatus := globalEgressIP.Status
		requeue := c.onCreate(globalEgressIP)

		return checkGlobalEgressIPStatusChanged(&prevStatus, &globalEgressIP.Status, globalEgressIP), requeue
	case syncer.Update:
		// TODO handle update
	case syncer.Delete:
		// TODO handle delete
		return nil, false
	}

	return nil, false
}

func (c *globalEgressIPController) onCreate(globalEgressIP *submarinerv1.GlobalEgressIP) bool {
	key, _ := cache.MetaNamespaceKeyFunc(globalEgressIP)

	requeue := allocateIPs(key, globalEgressIP.Spec.NumberOfIPs, c.pool, &globalEgressIP.Status)
	if requeue {
		return requeue
	}

	// TODO - add IP table rules for the allocated IPs?

	// TODO - start pod watcher?

	return false
}

func allocateIPs(key string, numberOfIPs *int, pool *IPPool, status *submarinerv1.GlobalEgressIPStatus) bool {
	if numberOfIPs == nil {
		one := 1
		numberOfIPs = &one
	}

	if *numberOfIPs == len(status.AllocatedIPs) {
		return false
	}

	// TODO - remove IP table rules for previous allocated IPs?

	klog.Infof("Allocating %d global IP(s) for %q", *numberOfIPs, key)

	status.AllocatedIPs = make([]string, 0, *numberOfIPs)

	for i := 0; i < *numberOfIPs; i++ {
		ip, err := pool.Allocate(key)
		if err != nil {
			klog.Errorf("Error allocating IPs for %q: %v", key, err)
			tryAppendStatusCondition(status, &metav1.Condition{
				Type:    string(submarinerv1.GlobalEgressIPAllocated),
				Status:  metav1.ConditionFalse,
				Reason:  "IPPoolAllocationFailed",
				Message: fmt.Sprintf("Error allocating %d global IP(s) from the pool: %v", numberOfIPs, err),
			})

			return true
		}

		status.AllocatedIPs = append(status.AllocatedIPs, ip)
	}

	tryAppendStatusCondition(status, &metav1.Condition{
		Type:    string(submarinerv1.GlobalEgressIPAllocated),
		Status:  metav1.ConditionTrue,
		Message: fmt.Sprintf("Allocated %d global IP(s)", numberOfIPs),
	})

	return false
}

func tryAppendStatusCondition(status *submarinerv1.GlobalEgressIPStatus, newCond *metav1.Condition) {
	updatedConditions := util.TryAppendCondition(status.Conditions, *newCond)
	if updatedConditions == nil {
		return
	}

	status.Conditions = updatedConditions
}

func checkGlobalEgressIPStatusChanged(oldStatus, newStatus *submarinerv1.GlobalEgressIPStatus, retObj runtime.Object) runtime.Object {
	if equality.Semantic.DeepEqual(oldStatus, newStatus) {
		return nil
	}

	klog.V(log.DEBUG).Infof("Updated GlobalEgressIPStatus: %#v", newStatus)

	return retObj
}

// TODO - use the function in admiral
func areSpecsEquivalent(obj1, obj2 *unstructured.Unstructured) bool {
	spec1, _, _ := unstructured.NestedMap(obj1.Object, "spec")
	spec2, _, _ := unstructured.NestedMap(obj2.Object, "spec")

	return reflect.DeepEqual(spec1, spec2)
}
