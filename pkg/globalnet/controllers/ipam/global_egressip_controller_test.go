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
package ipam_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/syncer"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	"github.com/submariner-io/submariner/pkg/globalnet/controllers/ipam"
)

var _ = Describe("GlobalEgressIP controller", func() {
	t := newGlobalEgressIPControllerTestDriver()

	When("a GlobalEgressIP is created", func() {
		var numberOfIPs int

		JustBeforeEach(func() {
			t.createGlobalEgressIP(newGlobalEgressIP(globalEgressIPName, numberOfIPs, nil))
		})

		Context("with the NumberOfIPs unspecified", func() {
			BeforeEach(func() {
				numberOfIPs = -1
			})

			It("should successfully allocate one global IP", func() {
				t.awaitGlobalEgressIPStatusAllocated(globalEgressIPName, 1)
			})
		})

		Context("with the NumberOfIPs specified", func() {
			BeforeEach(func() {
				numberOfIPs = 10
			})

			It("should successfully allocate the specified number of global IPs", func() {
				t.awaitGlobalEgressIPStatusAllocated(globalEgressIPName, numberOfIPs)
			})
		})
	})

	When("a GlobalEgressIP exists on startup", func() {
		var existing *submarinerv1.GlobalEgressIP

		BeforeEach(func() {
			existing = newGlobalEgressIP(globalEgressIPName, 1, nil)
			existing.Status.AllocatedIPs = []string{"169.254.1.1"}
			t.createGlobalEgressIP(existing)
		})

		It("should not reallocate the global IPs", func() {
			Consistently(func() []string {
				return getGlobalEgressIPStatus(t.globalEgressIPs, existing.Name).AllocatedIPs
			}).Should(Equal(existing.Status.AllocatedIPs))
		})

		It("should not update the Status conditions", func() {
			Consistently(func() int {
				return len(getGlobalEgressIPStatus(t.globalEgressIPs, existing.Name).Conditions)
			}).Should(Equal(0))
		})
	})
})

type globalEgressIPControllerTestDriver struct {
	*testDriverBase
}

func newGlobalEgressIPControllerTestDriver() *globalEgressIPControllerTestDriver {
	t := &globalEgressIPControllerTestDriver{}

	BeforeEach(func() {
		t.testDriverBase = newTestDriverBase()
	})

	JustBeforeEach(func() {
		t.start()
	})

	AfterEach(func() {
		t.testDriverBase.afterEach()
	})

	return t
}

func (t *globalEgressIPControllerTestDriver) start() {
	pool, err := ipam.NewIPPool(t.globalCIDR)
	Expect(err).To(Succeed())

	c, err := ipam.NewGlobalEgressIPController(syncer.ResourceSyncerConfig{
		SourceClient: t.dynClient,
		RestMapper:   t.restMapper,
		Scheme:       t.scheme,
	}, pool)

	Expect(err).To(Succeed())
	Expect(c.Start(t.stopCh)).To(Succeed())
}
