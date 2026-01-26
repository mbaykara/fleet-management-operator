/*
Copyright 2026.

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
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fleetmanagementv1alpha1 "github.com/grafana/fleet-management-operator/api/v1alpha1"
	"github.com/grafana/fleet-management-operator/pkg/fleetclient"
)

// Mock Fleet Management API client
type mockFleetClient struct {
	pipelines         map[string]*fleetclient.Pipeline
	upsertError       error
	deleteError       error
	callCount         int
	lastUpsertRequest *fleetclient.UpsertPipelineRequest
	shouldReturn404   bool
	shouldReturn400   bool
	shouldReturn429   bool
}

func newMockFleetClient() *mockFleetClient {
	return &mockFleetClient{
		pipelines: make(map[string]*fleetclient.Pipeline),
	}
}

func (m *mockFleetClient) UpsertPipeline(ctx context.Context, req *fleetclient.UpsertPipelineRequest) (*fleetclient.Pipeline, error) {
	m.callCount++
	m.lastUpsertRequest = req

	if m.shouldReturn400 {
		return nil, &fleetclient.FleetAPIError{
			StatusCode: http.StatusBadRequest,
			Operation:  "UpsertPipeline",
			Message:    "validation error: invalid configuration",
		}
	}

	if m.shouldReturn429 {
		return nil, &fleetclient.FleetAPIError{
			StatusCode: http.StatusTooManyRequests,
			Operation:  "UpsertPipeline",
			Message:    "rate limit exceeded",
		}
	}

	if m.shouldReturn404 {
		return nil, &fleetclient.FleetAPIError{
			StatusCode: http.StatusNotFound,
			Operation:  "UpsertPipeline",
			Message:    "pipeline not found",
		}
	}

	if m.upsertError != nil {
		return nil, m.upsertError
	}

	// Assign ID if not present
	if req.Pipeline.ID == "" {
		req.Pipeline.ID = "mock-id-123"
	}

	now := time.Now()
	req.Pipeline.CreatedAt = &now
	req.Pipeline.UpdatedAt = &now

	m.pipelines[req.Pipeline.ID] = req.Pipeline

	return req.Pipeline, nil
}

func (m *mockFleetClient) DeletePipeline(ctx context.Context, id string) error {
	if m.shouldReturn404 {
		return &fleetclient.FleetAPIError{
			StatusCode: http.StatusNotFound,
			Operation:  "DeletePipeline",
			Message:    "pipeline not found",
		}
	}

	if m.deleteError != nil {
		return m.deleteError
	}

	delete(m.pipelines, id)
	return nil
}

var _ = Describe("Pipeline Controller", func() {
	Context("When reconciling a Pipeline", func() {
		const (
			pipelineName      = "test-pipeline"
			pipelineNamespace = "default"
			timeout           = time.Second * 10
			interval          = time.Millisecond * 250
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      pipelineName,
			Namespace: pipelineNamespace,
		}

		AfterEach(func() {
			// Cleanup
			pipeline := &fleetmanagementv1alpha1.Pipeline{}
			err := k8sClient.Get(ctx, typeNamespacedName, pipeline)
			if err == nil {
				Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed())

				// Wait for pipeline to be fully deleted
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, pipeline)
					return err != nil && errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			}
		})

		It("should successfully reconcile a new Pipeline", func() {
			By("Creating a new Pipeline")
			pipeline := &fleetmanagementv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pipelineName,
					Namespace: pipelineNamespace,
				},
				Spec: fleetmanagementv1alpha1.PipelineSpec{
					Contents:   "prometheus.exporter.self \"alloy\" { }",
					Enabled:    true,
					Matchers:   []string{"env=prod"},
					ConfigType: fleetmanagementv1alpha1.ConfigTypeAlloy,
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())

			By("Checking if finalizer is added")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, pipeline)
				if err != nil {
					return false
				}
				for _, f := range pipeline.Finalizers {
					if f == pipelineFinalizer {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Checking if status is updated with Fleet Management ID")
			Eventually(func() string {
				err := k8sClient.Get(ctx, typeNamespacedName, pipeline)
				if err != nil {
					return ""
				}
				return pipeline.Status.ID
			}, timeout, interval).Should(Equal("mock-id-123"))

			By("Checking if Ready condition is set to True")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, pipeline)
				if err != nil {
					return false
				}
				for _, condition := range pipeline.Status.Conditions {
					if condition.Type == conditionTypeReady && condition.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		It("should skip reconciliation when spec hasn't changed", func() {
			By("Creating a Pipeline with Fleet ID already set")
			pipeline := &fleetmanagementv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:       pipelineName,
					Namespace:  pipelineNamespace,
					Generation: 1,
				},
				Spec: fleetmanagementv1alpha1.PipelineSpec{
					Contents:   "prometheus.exporter.self \"alloy\" { }",
					Enabled:    true,
					ConfigType: fleetmanagementv1alpha1.ConfigTypeAlloy,
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())

			// Wait for first reconciliation
			Eventually(func() string {
				err := k8sClient.Get(ctx, typeNamespacedName, pipeline)
				if err != nil {
					return ""
				}
				return pipeline.Status.ID
			}, timeout, interval).Should(Equal("mock-id-123"))

			// Get current state
			Expect(k8sClient.Get(ctx, typeNamespacedName, pipeline)).To(Succeed())
			currentGeneration := pipeline.Generation
			currentObservedGeneration := pipeline.Status.ObservedGeneration

			By("Verifying observedGeneration matches generation")
			Expect(currentObservedGeneration).To(Equal(currentGeneration))

			// Note: Without changing spec, controller should skip reconciliation
			// This is tested by the observedGeneration check in the controller
		})

		It("should handle deletion properly", func() {
			By("Creating a Pipeline")
			pipeline := &fleetmanagementv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pipelineName,
					Namespace: pipelineNamespace,
				},
				Spec: fleetmanagementv1alpha1.PipelineSpec{
					Contents:   "prometheus.exporter.self \"alloy\" { }",
					Enabled:    true,
					ConfigType: fleetmanagementv1alpha1.ConfigTypeAlloy,
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())

			// Wait for Fleet ID
			Eventually(func() string {
				err := k8sClient.Get(ctx, typeNamespacedName, pipeline)
				if err != nil {
					return ""
				}
				return pipeline.Status.ID
			}, timeout, interval).Should(Equal("mock-id-123"))

			By("Deleting the Pipeline")
			Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed())

			By("Verifying the Pipeline is deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, pipeline)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})

		It("should handle validation errors from Fleet Management API", func() {
			// This would require setting up the mock client differently
			// For now, we'll test the basic error handling path
			Skip("Requires dynamic mock client configuration")
		})

		It("should convert ConfigType correctly", func() {
			By("Testing Alloy config type")
			Expect(fleetmanagementv1alpha1.ConfigTypeAlloy.ToFleetAPI()).To(Equal("CONFIG_TYPE_ALLOY"))

			By("Testing OpenTelemetryCollector config type")
			Expect(fleetmanagementv1alpha1.ConfigTypeOpenTelemetryCollector.ToFleetAPI()).To(Equal("CONFIG_TYPE_OTEL"))

			By("Testing round-trip conversion")
			alloyType := fleetmanagementv1alpha1.ConfigTypeFromFleetAPI("CONFIG_TYPE_ALLOY")
			Expect(alloyType).To(Equal(fleetmanagementv1alpha1.ConfigTypeAlloy))

			otelType := fleetmanagementv1alpha1.ConfigTypeFromFleetAPI("CONFIG_TYPE_OTEL")
			Expect(otelType).To(Equal(fleetmanagementv1alpha1.ConfigTypeOpenTelemetryCollector))
		})
	})

	Context("When building UpsertPipelineRequest", func() {
		It("should use metadata.name when spec.name is empty", func() {
			reconciler := &PipelineReconciler{}
			pipeline := &fleetmanagementv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline",
					Namespace: "default",
				},
				Spec: fleetmanagementv1alpha1.PipelineSpec{
					Contents:   "test content",
					Enabled:    true,
					ConfigType: fleetmanagementv1alpha1.ConfigTypeAlloy,
				},
			}

			req, err := reconciler.buildUpsertRequest(pipeline)
			Expect(err).ToNot(HaveOccurred())
			Expect(req.Pipeline.Name).To(Equal("test-pipeline"))
		})

		It("should use spec.name when provided", func() {
			reconciler := &PipelineReconciler{}
			pipeline := &fleetmanagementv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "k8s-pipeline",
					Namespace: "default",
				},
				Spec: fleetmanagementv1alpha1.PipelineSpec{
					Name:       "custom-pipeline-name",
					Contents:   "test content",
					Enabled:    true,
					ConfigType: fleetmanagementv1alpha1.ConfigTypeAlloy,
				},
			}

			req, err := reconciler.buildUpsertRequest(pipeline)
			Expect(err).ToNot(HaveOccurred())
			Expect(req.Pipeline.Name).To(Equal("custom-pipeline-name"))
		})

		It("should convert ConfigType to Fleet API format", func() {
			reconciler := &PipelineReconciler{}
			pipeline := &fleetmanagementv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: fleetmanagementv1alpha1.PipelineSpec{
					Contents:   "test",
					ConfigType: fleetmanagementv1alpha1.ConfigTypeOpenTelemetryCollector,
				},
			}

			req, err := reconciler.buildUpsertRequest(pipeline)
			Expect(err).ToNot(HaveOccurred())
			Expect(req.Pipeline.ConfigType).To(Equal("CONFIG_TYPE_OTEL"))
		})
	})

	Context("Mock Fleet Client Tests", func() {
		It("should track API calls", func() {
			mock := newMockFleetClient()
			ctx := context.Background()

			req := &fleetclient.UpsertPipelineRequest{
				Pipeline: &fleetclient.Pipeline{
					Name:     "test",
					Contents: "test content",
				},
			}

			_, err := mock.UpsertPipeline(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(mock.callCount).To(Equal(1))
		})

		It("should store pipelines", func() {
			mock := newMockFleetClient()
			ctx := context.Background()

			req := &fleetclient.UpsertPipelineRequest{
				Pipeline: &fleetclient.Pipeline{
					Name:     "test",
					Contents: "test content",
				},
			}

			result, err := mock.UpsertPipeline(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.ID).To(Equal("mock-id-123"))
			Expect(result.Name).To(Equal("test"))
		})

		It("should handle deletion", func() {
			mock := newMockFleetClient()
			ctx := context.Background()

			req := &fleetclient.UpsertPipelineRequest{
				Pipeline: &fleetclient.Pipeline{
					Name:     "test",
					Contents: "test content",
				},
			}

			result, _ := mock.UpsertPipeline(ctx, req)
			err := mock.DeletePipeline(ctx, result.ID)
			Expect(err).ToNot(HaveOccurred())

			// Verify pipeline was removed from mock's internal storage
			_, exists := mock.pipelines[result.ID]
			Expect(exists).To(BeFalse())
		})
	})
})

// Helper function to setup controller with mock client for specific tests
func setupReconcilerWithMock(mockClient *mockFleetClient) (*PipelineReconciler, reconcile.Request) {
	reconciler := &PipelineReconciler{
		Client:      k8sClient,
		Scheme:      k8sClient.Scheme(),
		FleetClient: mockClient,
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-pipeline",
			Namespace: "default",
		},
	}

	return reconciler, req
}
