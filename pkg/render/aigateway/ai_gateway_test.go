// Copyright (c) 2026 Tigera, Inc. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package aigateway_test

import (
	"testing"

	operatorv1 "github.com/tigera/operator/api/v1"
	"github.com/tigera/operator/pkg/components"
	"github.com/tigera/operator/pkg/render/aigateway"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRenderDefaults(t *testing.T) {
	c := aigateway.NewComponent(&aigateway.Config{
		Installation: &operatorv1.InstallationSpec{Variant: operatorv1.TigeraSecureEnterprise},
		AIGateway:    &operatorv1.AIGateway{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	})

	// ResolveImages must succeed
	if err := c.ResolveImages(nil); err != nil {
		t.Fatalf("ResolveImages: %v", err)
	}

	objs, _ := c.Objects()

	var deploy *appsv1.Deployment
	for _, o := range objs {
		if d, ok := o.(*appsv1.Deployment); ok && d.Name == aigateway.ControllerName {
			deploy = d
			break
		}
	}
	if deploy == nil {
		t.Fatalf("controller Deployment %q not rendered", aigateway.ControllerName)
	}
	if deploy.Namespace != aigateway.ControllerNamespace {
		t.Errorf("namespace: got %q want %q", deploy.Namespace, aigateway.ControllerNamespace)
	}

	var svc *corev1.Service
	for _, o := range objs {
		if s, ok := o.(*corev1.Service); ok && s.Name == aigateway.ControllerName {
			svc = s
			break
		}
	}
	if svc == nil {
		t.Fatalf("controller Service not rendered")
	}
	foundPort := false
	for _, p := range svc.Spec.Ports {
		if p.Port == aigateway.ExtensionServerPort {
			foundPort = true
		}
	}
	if !foundPort {
		t.Errorf("Service missing port %d", aigateway.ExtensionServerPort)
	}

	hasRL := false
	for _, o := range objs {
		if d, ok := o.(*appsv1.Deployment); ok && d.Name == "envoy-ai-gateway-ratelimit" {
			hasRL = true
		}
	}
	if !hasRL {
		t.Errorf("rate-limit Deployment missing on default render")
	}
}

func TestRenderControllerDeploymentCustomization(t *testing.T) {
	replicas := int32(3)
	tol := []corev1.Toleration{{Key: "dedicated", Operator: corev1.TolerationOpExists}}
	nodeSel := map[string]string{"workload": "ai"}
	c := aigateway.NewComponent(&aigateway.Config{
		Installation: &operatorv1.InstallationSpec{Variant: operatorv1.TigeraSecureEnterprise},
		AIGateway: &operatorv1.AIGateway{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec: operatorv1.AIGatewaySpec{
				AIGatewayControllerDeployment: &operatorv1.AIGatewayControllerDeployment{
					Spec: &operatorv1.AIGatewayControllerDeploymentSpec{
						Replicas: &replicas,
						Template: &operatorv1.AIGatewayControllerDeploymentPodTemplateSpec{
							Spec: &operatorv1.AIGatewayControllerDeploymentPodSpec{
								Tolerations:  tol,
								NodeSelector: nodeSel,
							},
						},
					},
				},
			},
		},
	})
	if err := c.ResolveImages(nil); err != nil {
		t.Fatalf("ResolveImages: %v", err)
	}
	objs, _ := c.Objects()
	var deploy *appsv1.Deployment
	for _, o := range objs {
		if d, ok := o.(*appsv1.Deployment); ok && d.Name == aigateway.ControllerName {
			deploy = d
		}
	}
	if deploy == nil {
		t.Fatal("controller Deployment missing")
	}
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 3 {
		t.Errorf("replicas: got %v want 3", deploy.Spec.Replicas)
	}
	if len(deploy.Spec.Template.Spec.Tolerations) != 1 || deploy.Spec.Template.Spec.Tolerations[0].Key != "dedicated" {
		t.Errorf("tolerations not merged: %+v", deploy.Spec.Template.Spec.Tolerations)
	}
	if v := deploy.Spec.Template.Spec.NodeSelector["workload"]; v != "ai" {
		t.Errorf("nodeSelector not merged: %+v", deploy.Spec.Template.Spec.NodeSelector)
	}
}

func TestRenderRateLimitDisabled(t *testing.T) {
	c := aigateway.NewComponent(&aigateway.Config{
		Installation: &operatorv1.InstallationSpec{Variant: operatorv1.TigeraSecureEnterprise},
		AIGateway: &operatorv1.AIGateway{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec: operatorv1.AIGatewaySpec{
				RateLimitDeployment: &operatorv1.RateLimitDeployment{Disabled: true},
			},
		},
	})
	if err := c.ResolveImages(nil); err != nil {
		t.Fatalf("ResolveImages: %v", err)
	}
	objs, _ := c.Objects()
	for _, o := range objs {
		if d, ok := o.(*appsv1.Deployment); ok && d.Name == "envoy-ai-gateway-ratelimit" {
			t.Fatalf("rate-limit Deployment present when Disabled=true")
		}
	}
}

func TestRenderRBAC(t *testing.T) {
	c := aigateway.NewComponent(&aigateway.Config{
		Installation: &operatorv1.InstallationSpec{Variant: operatorv1.TigeraSecureEnterprise},
		AIGateway:    &operatorv1.AIGateway{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	})
	if err := c.ResolveImages(nil); err != nil {
		t.Fatalf("ResolveImages: %v", err)
	}
	objs, _ := c.Objects()
	var role *rbacv1.ClusterRole
	var binding *rbacv1.ClusterRoleBinding
	for _, o := range objs {
		switch x := o.(type) {
		case *rbacv1.ClusterRole:
			if x.Name == aigateway.ControllerName {
				role = x
			}
		case *rbacv1.ClusterRoleBinding:
			if x.Name == aigateway.ControllerName {
				binding = x
			}
		}
	}
	if role == nil {
		t.Fatal("ClusterRole not rendered")
	}
	if binding == nil {
		t.Fatal("ClusterRoleBinding not rendered")
	}
	// Must have an aigateway.envoyproxy.io rule that includes aigatewayroutes
	hasAIGRule := false
	for _, r := range role.Rules {
		hasGroup := false
		for _, g := range r.APIGroups {
			if g == "aigateway.envoyproxy.io" {
				hasGroup = true
				break
			}
		}
		if !hasGroup {
			continue
		}
		for _, res := range r.Resources {
			if res == "aigatewayroutes" {
				hasAIGRule = true
			}
		}
	}
	if !hasAIGRule {
		t.Error("ClusterRole missing aigateway.envoyproxy.io aigatewayroutes rule")
	}
	if binding.RoleRef.Name != aigateway.ControllerName {
		t.Errorf("binding.RoleRef.Name: got %q want %q", binding.RoleRef.Name, aigateway.ControllerName)
	}
	if len(binding.Subjects) != 1 || binding.Subjects[0].Name != aigateway.ControllerName ||
		binding.Subjects[0].Namespace != aigateway.ControllerNamespace {
		t.Errorf("binding subjects wrong: %+v", binding.Subjects)
	}
}

func TestRenderCRDs(t *testing.T) {
	c := aigateway.NewComponent(&aigateway.Config{
		Installation: &operatorv1.InstallationSpec{Variant: operatorv1.TigeraSecureEnterprise},
		AIGateway:    &operatorv1.AIGateway{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	})
	if err := c.ResolveImages(nil); err != nil {
		t.Fatalf("ResolveImages: %v", err)
	}
	objs, _ := c.Objects()
	want := map[string]bool{
		"aigatewayroutes.aigateway.envoyproxy.io":         false,
		"aiservicebackends.aigateway.envoyproxy.io":       false,
		"backendsecuritypolicies.aigateway.envoyproxy.io": false,
	}
	for _, o := range objs {
		if crd, ok := o.(*apiextv1.CustomResourceDefinition); ok {
			if _, found := want[crd.Name]; found {
				want[crd.Name] = true
			}
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("CRD %s not rendered", name)
		}
	}
}

// TestUpstreamImagesBypassRegistry proves the HACK(hackathon) pin: even with a
// non-empty Installation.Registry/ImagePath (which would normally compose a
// Tigera-registry ref), the controller Deployment and the extproc sidecar must
// emit the literal upstream docker.io/envoyproxy refs so the demo can pull them.
func TestUpstreamImagesBypassRegistry(t *testing.T) {
	const wantController = "docker.io/envoyproxy/ai-gateway-controller:v0.7.0"
	const wantExtProc = "docker.io/envoyproxy/ai-gateway-extproc:v0.7.0"

	// Constants must be the exact upstream refs.
	if aigateway.UpstreamControllerImage != wantController {
		t.Errorf("UpstreamControllerImage: got %q want %q", aigateway.UpstreamControllerImage, wantController)
	}
	if aigateway.UpstreamExtProcImage != wantExtProc {
		t.Errorf("UpstreamExtProcImage: got %q want %q", aigateway.UpstreamExtProcImage, wantExtProc)
	}

	// Render with a Tigera-style registry/path set to prove it is bypassed.
	c := aigateway.NewComponent(&aigateway.Config{
		Installation: &operatorv1.InstallationSpec{
			Variant:   operatorv1.TigeraSecureEnterprise,
			Registry:  "quay.io/",
			ImagePath: "tigera",
		},
		AIGateway: &operatorv1.AIGateway{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	})
	if err := c.ResolveImages(nil); err != nil {
		t.Fatalf("ResolveImages: %v", err)
	}
	objs, _ := c.Objects()

	var deploy *appsv1.Deployment
	for _, o := range objs {
		if d, ok := o.(*appsv1.Deployment); ok && d.Name == aigateway.ControllerName {
			deploy = d
			break
		}
	}
	if deploy == nil {
		t.Fatalf("controller Deployment %q not rendered", aigateway.ControllerName)
	}
	if got := deploy.Spec.Template.Spec.Containers[0].Image; got != wantController {
		t.Errorf("controller image: got %q want %q (registry composition not bypassed)", got, wantController)
	}

	// The extproc sidecar image is consumed by the gatewayapi render via the
	// same upstream constant; assert the helper that builds the sidecar emits it.
	extProc := aigateway.ExtProcContainer(aigateway.UpstreamExtProcImage, nil)
	if extProc.Image != wantExtProc {
		t.Errorf("extproc sidecar image: got %q want %q", extProc.Image, wantExtProc)
	}
}

func TestVersionStampInvariant(t *testing.T) {
	if components.ComponentGatewayAPIAIGatewayController.Version !=
		components.ComponentGatewayAPIAIExtProc.Version {
		t.Fatalf("version mismatch: controller=%s extproc=%s — would break filterapi.Config.Version handshake",
			components.ComponentGatewayAPIAIGatewayController.Version,
			components.ComponentGatewayAPIAIExtProc.Version)
	}
}
