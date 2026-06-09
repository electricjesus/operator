// Copyright (c) 2026 Tigera, Inc. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package aigateway_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorv1 "github.com/tigera/operator/api/v1"
	"github.com/tigera/operator/pkg/controller/aigateway"
)

// fakeStatusManager implements status.StatusManager for tests.
type fakeStatusManager struct {
	degraded  bool
	available bool
	found     bool
}

func (f *fakeStatusManager) Run(_ context.Context)                                            {}
func (f *fakeStatusManager) OnCRFound()                                                       { f.found = true }
func (f *fakeStatusManager) OnCRNotFound()                                                    { f.found = false }
func (f *fakeStatusManager) AddDaemonsets(_ []types.NamespacedName)                           {}
func (f *fakeStatusManager) AddDeployments(_ []types.NamespacedName)                          {}
func (f *fakeStatusManager) AddStatefulSets(_ []types.NamespacedName)                         {}
func (f *fakeStatusManager) AddCronJobs(_ []types.NamespacedName)                             {}
func (f *fakeStatusManager) AddCertificateSigningRequests(_ string, _ map[string]string)      {}
func (f *fakeStatusManager) RemoveDaemonsets(_ ...types.NamespacedName)                       {}
func (f *fakeStatusManager) RemoveDeployments(_ ...types.NamespacedName)                      {}
func (f *fakeStatusManager) RemoveStatefulSets(_ ...types.NamespacedName)                     {}
func (f *fakeStatusManager) RemoveCronJobs(_ ...types.NamespacedName)                         {}
func (f *fakeStatusManager) RemoveCertificateSigningRequests(_ string)                        {}
func (f *fakeStatusManager) SetDegraded(_ operatorv1.TigeraStatusReason, _ string, _ error, _ logr.Logger) {
	f.degraded = true
}
func (f *fakeStatusManager) ClearDegraded()                  { f.degraded = false }
func (f *fakeStatusManager) SetWarning(_ string, _ string)   {}
func (f *fakeStatusManager) ClearWarning(_ string)           {}
func (f *fakeStatusManager) IsAvailable() bool               { return f.available }
func (f *fakeStatusManager) IsProgressing() bool             { return false }
func (f *fakeStatusManager) IsDegraded() bool                { return f.degraded }
func (f *fakeStatusManager) ReadyToMonitor()                 {}
func (f *fakeStatusManager) SetMetaData(_ *metav1.ObjectMeta) {}

func TestReconcileNoCR(t *testing.T) {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = operatorv1.AddToScheme(s)
	cli := fake.NewClientBuilder().WithScheme(s).Build()
	r := aigateway.NewReconciler(cli, s)
	res, err := r.Reconcile(context.Background(), reconcile.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requeue {
		t.Error("requeue requested without a CR")
	}
}

func TestVariantGate(t *testing.T) {
	s := runtime.NewScheme()
	_ = operatorv1.AddToScheme(s)
	cli := fake.NewClientBuilder().WithScheme(s).
		WithObjects(
			&operatorv1.Installation{
				ObjectMeta: metav1.ObjectMeta{Name: "default"},
				Spec:       operatorv1.InstallationSpec{Variant: operatorv1.Calico},
			},
			&operatorv1.AIGateway{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		).Build()
	r := aigateway.NewReconciler(cli, s)
	statusM := &fakeStatusManager{}
	r.SetStatus(statusM)
	_, err := r.Reconcile(context.Background(), reconcile.Request{})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !statusM.degraded {
		t.Errorf("expected Degraded for non-EE variant")
	}
}

func TestGatewayAPIMissingDegraded(t *testing.T) {
	s := runtime.NewScheme()
	_ = operatorv1.AddToScheme(s)
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(
		&operatorv1.Installation{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec:       operatorv1.InstallationSpec{Variant: operatorv1.TigeraSecureEnterprise},
		},
		&operatorv1.AIGateway{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	).Build()
	r := aigateway.NewReconciler(cli, s)
	statusM := &fakeStatusManager{}
	r.SetStatus(statusM)
	_, _ = r.Reconcile(context.Background(), reconcile.Request{})
	if !statusM.degraded {
		t.Errorf("expected Degraded when GatewayAPI is missing")
	}
}

func TestRenderAndApply(t *testing.T) {
	s := runtime.NewScheme()
	_ = operatorv1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	_ = rbacv1.AddToScheme(s)
	_ = apiextv1.AddToScheme(s)
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(
		&operatorv1.Installation{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec:       operatorv1.InstallationSpec{Variant: operatorv1.TigeraSecureEnterprise},
		},
		&operatorv1.GatewayAPI{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec: operatorv1.GatewayAPISpec{
				GatewayClasses: []operatorv1.GatewayClassSpec{{Name: "tigera-gateway-class"}},
			},
		},
		&operatorv1.AIGateway{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	).Build()
	r := aigateway.NewReconciler(cli, s)
	r.SetStatus(&fakeStatusManager{})
	if _, err := r.Reconcile(context.Background(), reconcile.Request{}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	d := &appsv1.Deployment{}
	if err := cli.Get(context.Background(),
		types.NamespacedName{Name: "envoy-ai-gateway-controller", Namespace: "calico-system"},
		d); err != nil {
		t.Fatalf("controller Deployment not applied: %v", err)
	}
}

func TestFinalizerAdded(t *testing.T) {
	s := runtime.NewScheme()
	_ = operatorv1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	_ = rbacv1.AddToScheme(s)
	_ = apiextv1.AddToScheme(s)
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(
		&operatorv1.Installation{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec:       operatorv1.InstallationSpec{Variant: operatorv1.TigeraSecureEnterprise},
		},
		&operatorv1.GatewayAPI{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec: operatorv1.GatewayAPISpec{
				GatewayClasses: []operatorv1.GatewayClassSpec{{Name: "tigera-gateway-class"}},
			},
		},
		&operatorv1.AIGateway{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	).Build()
	r := aigateway.NewReconciler(cli, s)
	r.SetStatus(&fakeStatusManager{})
	if _, err := r.Reconcile(context.Background(), reconcile.Request{}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	d := &appsv1.Deployment{}
	if err := cli.Get(context.Background(),
		types.NamespacedName{Name: "envoy-ai-gateway-controller", Namespace: "calico-system"},
		d); err != nil {
		t.Fatalf("controller Deployment not applied: %v", err)
	}
	found := false
	for _, f := range d.Finalizers {
		if f == "operator.tigera.io/aigateway-controller" {
			found = true
		}
	}
	if !found {
		t.Errorf("finalizer %q not added; finalizers=%v",
			"operator.tigera.io/aigateway-controller", d.Finalizers)
	}
}

func TestPeriodicRequeue(t *testing.T) {
	s := runtime.NewScheme()
	_ = operatorv1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	_ = rbacv1.AddToScheme(s)
	_ = apiextv1.AddToScheme(s)
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(
		&operatorv1.Installation{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec:       operatorv1.InstallationSpec{Variant: operatorv1.TigeraSecureEnterprise},
		},
		&operatorv1.GatewayAPI{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec: operatorv1.GatewayAPISpec{
				GatewayClasses: []operatorv1.GatewayClassSpec{{Name: "tigera-gateway-class"}},
			},
		},
		&operatorv1.AIGateway{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	).Build()
	r := aigateway.NewReconciler(cli, s)
	r.SetStatus(&fakeStatusManager{})
	res, err := r.Reconcile(context.Background(), reconcile.Request{})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter != 10*time.Minute {
		t.Errorf("RequeueAfter: got %v want %v", res.RequeueAfter, 10*time.Minute)
	}
}

func TestUnknownGatewayClassDegraded(t *testing.T) {
	s := runtime.NewScheme()
	_ = operatorv1.AddToScheme(s)
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(
		&operatorv1.Installation{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec:       operatorv1.InstallationSpec{Variant: operatorv1.TigeraSecureEnterprise},
		},
		&operatorv1.GatewayAPI{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec: operatorv1.GatewayAPISpec{
				GatewayClasses: []operatorv1.GatewayClassSpec{{Name: "tigera-gateway-class"}},
			},
		},
		&operatorv1.AIGateway{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec:       operatorv1.AIGatewaySpec{GatewayClasses: []string{"nonexistent"}},
		},
	).Build()
	r := aigateway.NewReconciler(cli, s)
	statusM := &fakeStatusManager{}
	r.SetStatus(statusM)
	_, _ = r.Reconcile(context.Background(), reconcile.Request{})
	if !statusM.degraded {
		t.Errorf("expected Degraded for unknown gatewayClasses entry")
	}
}
