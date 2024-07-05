/*
Copyright 2024.

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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	elasticwebv1 "github.com/zhaohaihang/elasticweb/api/v1"
)

var _ = Describe("ElasticWeb Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		elasticweb := &elasticwebv1.ElasticWeb{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ElasticWeb")
			err := k8sClient.Get(ctx, typeNamespacedName, elasticweb)
			if err != nil && errors.IsNotFound(err) {
				resource := &elasticwebv1.ElasticWeb{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &elasticwebv1.ElasticWeb{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ElasticWeb")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ElasticWebReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})

var _ = Describe("ElasticWeb controller", func() {
	Context("When updating ElasticWeb Status", func() {
		It("Should Create ElasticWeb ", func() {
			By("By creating a new ElasticWeb")
			ctx := context.Background()
			elasticWeb := &elasticwebv1.ElasticWeb{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "",
					Kind:       "",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-elasticweb",
					Namespace: "dev",
				},
				Spec: elasticwebv1.ElasticWebSpec{
					Image:        "nginx:1.7.9",
					Port:         8082,
					SinglePodQPS: 400,
					TotalQPS:     800,
				},
			}
			Expect(k8sClient.Create(ctx, elasticWeb)).Should(Succeed())
		})
	})
})
