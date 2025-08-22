//go:build integration

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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/kanisterio/datamover/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("DatamoverSession Controller", func() {
	Context("When reconciling a resource without lifecycle", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		datamoversession := &api.DatamoverSession{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind DatamoverSession")
			err := k8sClient.Get(ctx, typeNamespacedName, datamoversession)
			if err != nil && errors.IsNotFound(err) {
				resource := &api.DatamoverSession{
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
			resource := &api.DatamoverSession{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance DatamoverSession")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource without any changes", func() {
			By("Reconciling the created resource")

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Not updating status")
			resource := &api.DatamoverSession{}
			err = k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(resource.Status.Progress).To(Equal(api.ProgressNone))

			By("Not creating any resources")
			pod, err := controllerReconciler.getPod(ctx, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(pod).To(BeNil())

			service, err := controllerReconciler.getService(ctx, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(service).To(BeNil())
		})
	})

	Context("When reconciling a resource with lifecycle", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		datamoversession := &api.DatamoverSession{}

		var resource *api.DatamoverSession

		JustBeforeEach(func() {
			By("creating the custom resource for the Kind DatamoverSession")
			err := k8sClient.Get(ctx, typeNamespacedName, datamoversession)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup all child resources")
			resources, err := controllerReconciler.getResources(ctx, resource)
			controllerReconciler.CleanupService(ctx, resource, resources)
			controllerReconciler.CleanupPod(ctx, resource, resources)

			By("Cleanup the specific resource instance DatamoverSession")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		Describe("When spec is invalid", func() {
			BeforeEach(func() {
				By("Configuring invalid resource")
				resource = &api.DatamoverSession{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: api.DatamoverSessionSpec{
						LifecycleConfig: &api.LifecycleConfig{},
					},
				}
			})
			It("should successfully reconcile", func() {
				By("Reconciling the created resource")

				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())

				By("Setting status to invalid")
				resource := &api.DatamoverSession{}
				err = k8sClient.Get(ctx, typeNamespacedName, resource)
				Expect(err).NotTo(HaveOccurred())
				Expect(resource.Status.Progress).To(Equal(api.ProgressValidationFailed))

				By("Not creating any resources")
				pod, err := controllerReconciler.getPod(ctx, resource)
				Expect(err).NotTo(HaveOccurred())
				Expect(pod).To(BeNil())

				service, err := controllerReconciler.getService(ctx, resource)
				Expect(err).NotTo(HaveOccurred())
				Expect(service).To(BeNil())

				By("Reconciling again")
				result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())

				By("Not updating status")
				resource = &api.DatamoverSession{}
				err = k8sClient.Get(ctx, typeNamespacedName, resource)
				Expect(err).NotTo(HaveOccurred())
				Expect(resource.Status.Progress).To(Equal(api.ProgressValidationFailed))

			})
		})

		Describe("When spec is valid", func() {
			When("There is no ports", func() {
				BeforeEach(func() {
					By("Configuring valid resource")
					resource = &api.DatamoverSession{
						ObjectMeta: metav1.ObjectMeta{
							Name:      resourceName,
							Namespace: "default",
						},
						Spec: api.DatamoverSessionSpec{
							Implementation: "noop",
							LifecycleConfig: &api.LifecycleConfig{
								Image: "datamover/noop-session:dev",
								PodOptions: api.PodOptions{
									PodOverride: api.PodOverride{
										"containers": []map[string]interface{}{{
											"name":  api.DefaultContainerName,
											"image": "busybox:latest",
											"command": []string{
												"sh",
												"-c",
												"echo foo > /etc/session/ready && while [ -f /etc/session/ready ]; do sleep 1; done; exit 1",
											},
										}},
									},
								},
							},
						},
					}
				})
				It("should successfully reconcile", func() {
					By("Reconciling the created resource")

					result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Setting result to requeue")
					Expect(result.Requeue).To(BeTrue())

					By("Not Expecting the status to be set")
					resource := &api.DatamoverSession{}
					err = k8sClient.Get(ctx, typeNamespacedName, resource)
					Expect(err).NotTo(HaveOccurred())
					Expect(resource.Status.Progress).To(Equal(api.ProgressNone))

					By("Creating a pod")
					pod, err := controllerReconciler.getPod(ctx, resource)
					Expect(err).NotTo(HaveOccurred())
					Expect(pod).To(Not(BeNil()))

					By("Not creating a service")
					service, err := controllerReconciler.getService(ctx, resource)
					Expect(err).NotTo(HaveOccurred())
					Expect(service).To(BeNil())
				})
				When("Reconciled more times", func() {
					It("should successfully reconcile", func() {
						By("Reconciling the created resource once")
						result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())
						By("Setting result to requeue")
						Expect(result.Requeue).To(BeTrue())

						By("Reconciling while waiting for resources")
						result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())
						By("Setting result to not requeue")
						Expect(result.Requeue).To(BeFalse())

						By("Expecting the status to be set to ResourcesCreated")
						resource := &api.DatamoverSession{}
						err = k8sClient.Get(ctx, typeNamespacedName, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(resource.Status.Progress).To(Equal(api.ProgressResourcesCreated))

						By("Reconciling while waiting for readiness")
						result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())
						By("Setting result to requeue")
						Expect(result.Requeue).To(BeTrue())

					})
				})
				When("Waiting for resources to be ready", func() {
					It("should successfully reconcile", func() {
						By("Reconciling the created resource once")
						result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())
						By("Setting result to requeue")
						Expect(result.Requeue).To(BeTrue())

						By("Reconciling while resources are creating")
						result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())
						By("Setting result to not requeue")
						Expect(result.Requeue).To(BeFalse())

						By("Waiting for pod to be ready")
						Eventually(func() bool {
							pod, err := controllerReconciler.getPod(ctx, resource)

							Expect(err).NotTo(HaveOccurred())
							Expect(pod).To(Not(BeNil()))
							readiness, err := controllerReconciler.getReadiness(ctx, *pod)
							Expect(err).NotTo(HaveOccurred())
							return readiness.ready
						}).WithPolling(1 * time.Second).WithTimeout(10 * time.Second).Should(BeTrue())

						pod, err := controllerReconciler.getPod(ctx, resource)

						Expect(err).NotTo(HaveOccurred())
						Expect(pod).To(Not(BeNil()))
						Expect(isPodReady(*pod)).To(BeTrue())

						By("Reconciling again")
						result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())
						By("Setting result to not requeue")
						Expect(result.Requeue).To(BeFalse())

						By("Expecting the status to be set to Ready")
						resource := &api.DatamoverSession{}
						err = k8sClient.Get(ctx, typeNamespacedName, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(resource.Status.Progress).To(Equal(api.ProgressReady))

						By("Reconciling again")
						result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())
						By("Setting result to not requeue")
						Expect(result.Requeue).To(BeFalse())

						By("Expecting the status to be set to Ready")
						resource = &api.DatamoverSession{}
						err = k8sClient.Get(ctx, typeNamespacedName, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(resource.Status.Progress).To(Equal(api.ProgressReady))
					})
				})
			})

			When("There there are ports", func() {
				BeforeEach(func() {
					By("Configuring valid resource")
					resource = &api.DatamoverSession{
						ObjectMeta: metav1.ObjectMeta{
							Name:      resourceName,
							Namespace: "default",
						},
						Spec: api.DatamoverSessionSpec{
							Implementation: "noop",
							LifecycleConfig: &api.LifecycleConfig{
								Image: "datamover/noop-session:dev",
								ServicePorts: []corev1.ServicePort{
									{Name: "something", Port: 2000},
								},
								PodOptions: api.PodOptions{
									PodOverride: api.PodOverride{
										"containers": []map[string]interface{}{{
											"name":  api.DefaultContainerName,
											"image": "busybox:latest",
											"command": []string{
												"sh",
												"-c",
												"echo foo > /etc/session/ready && while [ -f /etc/session/ready ]; do sleep 1; done; exit 1",
											},
										}},
									},
								},
							},
						},
					}
				})
				It("should successfully reconcile", func() {
					By("Reconciling the created resource")

					result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By("Setting result to requeue")
					Expect(result.Requeue).To(BeTrue())

					By("Not Expecting the status to be set")
					resource := &api.DatamoverSession{}
					err = k8sClient.Get(ctx, typeNamespacedName, resource)
					Expect(err).NotTo(HaveOccurred())
					Expect(resource.Status.Progress).To(Equal(api.ProgressNone))

					By("Creating a pod")
					pod, err := controllerReconciler.getPod(ctx, resource)
					Expect(err).NotTo(HaveOccurred())
					Expect(pod).To(Not(BeNil()))

					By("Creating a service")
					service, err := controllerReconciler.getService(ctx, resource)
					Expect(err).NotTo(HaveOccurred())
					Expect(service).To(Not(BeNil()))
				})
				When("Resources removed during creation", func() {
					It("should successfully reconcile", func() {
						By("Reconciling the created resource")

						result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())

						By("Setting result to requeue")
						Expect(result.Requeue).To(BeTrue())

						By("Not Expecting the status to be set")
						resource := &api.DatamoverSession{}
						err = k8sClient.Get(ctx, typeNamespacedName, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(resource.Status.Progress).To(Equal(api.ProgressNone))

						By("Creating a pod")
						pod, err := controllerReconciler.getPod(ctx, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(pod).To(Not(BeNil()))

						By("Creating a service")
						service, err := controllerReconciler.getService(ctx, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(service).To(Not(BeNil()))

						By("Service is deleted")
						err = controllerReconciler.DeleteService(ctx, service)
						Expect(err).NotTo(HaveOccurred())

						By("Reconciling on missing resources")
						result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())

						By("Setting result to not requeue")
						Expect(result.Requeue).To(BeFalse())

						By("Service is created")
						service, err = controllerReconciler.getService(ctx, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(service).To(Not(BeNil()))

						By("Pod is deleted")
						err = controllerReconciler.DeletePod(ctx, pod)
						Expect(err).NotTo(HaveOccurred())

						By("Waiting for pod to be deleted")
						Eventually(func() bool {
							pod, err := controllerReconciler.getPod(ctx, resource)
							Expect(err).NotTo(HaveOccurred())
							return pod == nil
						}).WithPolling(1 * time.Second).WithTimeout(10 * time.Second).Should(BeTrue())

						By("Reconciling on missing resources")
						result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())

						By("Setting result to not requeue")
						Expect(result.Requeue).To(BeFalse())

						By("Pod is created")
						pod, err = controllerReconciler.getPod(ctx, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(service).To(Not(BeNil()))

						By("Not Expecting the status to be set")
						resource = &api.DatamoverSession{}
						err = k8sClient.Get(ctx, typeNamespacedName, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(resource.Status.Progress).To(Equal(api.ProgressNone))
					})
				})

				When("Resources removed during readiness", func() {
					It("should successfully reconcile", func() {
						By("Reconciling the created resource")
						result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())

						By("Setting result to requeue")
						Expect(result.Requeue).To(BeTrue())

						By("Not Expecting the status to be set")
						resource := &api.DatamoverSession{}
						err = k8sClient.Get(ctx, typeNamespacedName, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(resource.Status.Progress).To(Equal(api.ProgressNone))

						By("Creating a pod")
						pod, err := controllerReconciler.getPod(ctx, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(pod).To(Not(BeNil()))

						By("Creating a service")
						service, err := controllerReconciler.getService(ctx, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(service).To(Not(BeNil()))

						By("Reconciling the created resource")
						result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())

						By("Setting result to not requeue")
						Expect(result.Requeue).To(BeFalse())

						By("Expecting the status to be set to ResourcesCreated")
						resource = &api.DatamoverSession{}
						err = k8sClient.Get(ctx, typeNamespacedName, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(resource.Status.Progress).To(Equal(api.ProgressResourcesCreated))

						By("Service deleted while waiting for ready")
						err = controllerReconciler.DeleteService(ctx, service)
						Expect(err).NotTo(HaveOccurred())

						By("Reconciling the created resource")
						result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())

						By("Expecting the status to be set to ReadinessFailure")
						resource = &api.DatamoverSession{}
						err = k8sClient.Get(ctx, typeNamespacedName, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(resource.Status.Progress).To(Equal(api.ProgressReadinessFailure))
					})
				})

				When("Failing on running session", func() {
					It("should successfully reconcile", func() {
						By("Reconciling the created resource once")
						result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())
						By("Setting result to requeue")
						Expect(result.Requeue).To(BeTrue())

						By("Reconciling while resources are creating")
						result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())
						By("Setting result to not requeue")
						Expect(result.Requeue).To(BeFalse())

						By("Service is there")
						service, err := controllerReconciler.getService(ctx, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(service).To(Not(BeNil()))

						By("Waiting for pod to be ready")
						Eventually(func() bool {
							pod, err := controllerReconciler.getPod(ctx, resource)

							Expect(err).NotTo(HaveOccurred())
							Expect(pod).To(Not(BeNil()))
							readiness, err := controllerReconciler.getReadiness(ctx, *pod)
							Expect(err).NotTo(HaveOccurred())
							return readiness.ready
						}).WithPolling(1 * time.Second).WithTimeout(10 * time.Second).Should(BeTrue())

						By("Reconciling while resources are ready")
						result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())

						By("Expecting the status to be set to Ready")
						resource := &api.DatamoverSession{}
						err = k8sClient.Get(ctx, typeNamespacedName, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(resource.Status.Progress).To(Equal(api.ProgressReady))

						By("On pod failure after readiness")
						pod, err := controllerReconciler.getPod(ctx, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(pod).To(Not(BeNil()))

						patch := client.MergeFrom(pod.DeepCopy())
						pod.Spec.EphemeralContainers = []corev1.EphemeralContainer{
							{
								EphemeralContainerCommon: corev1.EphemeralContainerCommon{
									Name: "terminator",
									// FIXME: image here?
									Image: "busybox:latest",
									// FIXME: config for fetching server data in the CRD
									Command: []string{"sh", "-c", "rm /etc/session/ready"},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "session-data",
											MountPath: "/etc/session",
										},
									},
								},
							},
						}
						err = controllerReconciler.SubResource("ephemeralcontainers").Patch(ctx, pod, patch)
						Expect(err).NotTo(HaveOccurred())

						By("Waiting for pod to fail")
						Eventually(func() bool {
							pod, err := controllerReconciler.getPod(ctx, resource)

							Expect(err).NotTo(HaveOccurred())
							Expect(pod).To(Not(BeNil()))
							return pod.Status.Phase == corev1.PodFailed
						}).WithPolling(1 * time.Second).WithTimeout(60 * time.Second).Should(BeTrue())

						By("Reconciling when pod failed")
						result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())

						By("Expecting the status to be set to SessionFailure")
						resource = &api.DatamoverSession{}
						err = k8sClient.Get(ctx, typeNamespacedName, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(resource.Status.Progress).To(Equal(api.ProgressSessionFailure))

						By("Service is still there")
						service, err = controllerReconciler.getService(ctx, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(service).To(Not(BeNil()))

						By("Reconciling after session failure")
						result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())

						By("Expecting the status to be set to SessionFailure")
						resource = &api.DatamoverSession{}
						err = k8sClient.Get(ctx, typeNamespacedName, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(resource.Status.Progress).To(Equal(api.ProgressSessionFailure))

						By("Service is deleted")
						service, err = controllerReconciler.getService(ctx, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(service).To(BeNil())

						By("Reconciling after session failure and resources deleted")
						result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())

						By("Expecting the status to be set to SessionFailure")
						resource = &api.DatamoverSession{}
						err = k8sClient.Get(ctx, typeNamespacedName, resource)
						Expect(err).NotTo(HaveOccurred())
						Expect(resource.Status.Progress).To(Equal(api.ProgressSessionFailure))
					})
				})
			})
		})
		Describe("When pod fails to start", func() {
			BeforeEach(func() {
				By("Configuring valid resource with failing pod")
				resource = &api.DatamoverSession{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: api.DatamoverSessionSpec{
						Implementation: "noop",
						LifecycleConfig: &api.LifecycleConfig{
							Image: "datamover/noop-session:dev",
							ServicePorts: []corev1.ServicePort{
								{Name: "something", Port: 2000},
							},
							PodOptions: api.PodOptions{
								PodOverride: api.PodOverride{
									"containers": []map[string]interface{}{{
										"name":    api.DefaultContainerName,
										"image":   "busybox:latest",
										"command": []string{"sh", "-c", "exit 1"},
									}},
								},
							},
						},
					},
				}
			})
			It("should successfully reconcile", func() {
				By("Reconciling the created resource")

				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Setting result to requeue")
				Expect(result.Requeue).To(BeTrue())

				By("Not Expecting the status to be set")
				resource := &api.DatamoverSession{}
				err = k8sClient.Get(ctx, typeNamespacedName, resource)
				Expect(err).NotTo(HaveOccurred())
				Expect(resource.Status.Progress).To(Equal(api.ProgressNone))

				By("Creating a pod")
				pod, err := controllerReconciler.getPod(ctx, resource)
				Expect(err).NotTo(HaveOccurred())
				Expect(pod).To(Not(BeNil()))

				By("Creating a service")
				service, err := controllerReconciler.getService(ctx, resource)
				Expect(err).NotTo(HaveOccurred())
				Expect(service).To(Not(BeNil()))

				By("Reconciling with resources created")
				result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Expecting the status to be set to ResourcesCreated")
				resource = &api.DatamoverSession{}
				err = k8sClient.Get(ctx, typeNamespacedName, resource)
				Expect(err).NotTo(HaveOccurred())
				Expect(resource.Status.Progress).To(Equal(api.ProgressResourcesCreated))

				By("Waiting for resource to fail")
				Eventually(func() bool {
					pod, err := controllerReconciler.getPod(ctx, resource)

					Expect(err).NotTo(HaveOccurred())
					Expect(pod).To(Not(BeNil()))
					return pod.Status.Phase == corev1.PodFailed
				}).WithPolling(1 * time.Second).WithTimeout(60 * time.Second).Should(BeTrue())

				By("Reconciling waiting for readiness")
				result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Expecting the status to be set to ReadinessFailure")
				resource = &api.DatamoverSession{}
				err = k8sClient.Get(ctx, typeNamespacedName, resource)
				Expect(err).NotTo(HaveOccurred())
				Expect(resource.Status.Progress).To(Equal(api.ProgressReadinessFailure))

				By("Service is still there")
				service, err = controllerReconciler.getService(ctx, resource)
				Expect(err).NotTo(HaveOccurred())
				Expect(service).To(Not(BeNil()))

				By("Reconciling waiting for cleanup")
				result, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Service should be deleted")
				service, err = controllerReconciler.getService(ctx, resource)
				Expect(err).NotTo(HaveOccurred())
				Expect(service).To(BeNil())
			})
		})
	})
})
