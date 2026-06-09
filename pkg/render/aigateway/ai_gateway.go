// Copyright (c) 2026 Tigera, Inc. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package aigateway

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"sync"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1 "github.com/tigera/operator/api/v1"
	"github.com/tigera/operator/pkg/render"
	rmeta "github.com/tigera/operator/pkg/render/common/meta"
)

//go:embed ai-gateway-crds-helm.tgz
var crdsChartBytes []byte

const (
	rateLimitName          = "envoy-ai-gateway-ratelimit"
	rateLimitContainerName = "ratelimit"
)

// Config wires the inputs the AI Gateway render needs.
type Config struct {
	Installation *operatorv1.InstallationSpec
	AIGateway    *operatorv1.AIGateway
	PullSecrets  []*corev1.Secret
}

type component struct {
	cfg *Config

	controllerImage string
	extProcImage    string
}

// NewComponent returns the render.Component that emits the AI Gateway
// controller Deployment+Service plus the rate-limit Deployment+Service.
func NewComponent(cfg *Config) render.Component {
	return &component{cfg: cfg}
}

func (c *component) ResolveImages(is *operatorv1.ImageSet) error {
	// HACK(hackathon): pin EAIG controller/extproc to upstream vendor images;
	// revert to Tigera-mirrored + normal registry composition post-hackathon.
	// EAIG images are not in the Tigera registry, so the normal
	// components.GetReference(...) composition resolves to an unpullable ref.
	// See UpstreamControllerImage / UpstreamExtProcImage in constants.go.
	c.controllerImage = UpstreamControllerImage
	c.extProcImage = UpstreamExtProcImage
	return nil
}

func (c *component) SupportedOSType() rmeta.OSType {
	return rmeta.OSTypeLinux
}

func (c *component) Ready() bool {
	return true
}

func (c *component) Objects() ([]client.Object, []client.Object) {
	objs := []client.Object{
		c.serviceAccount(),
		c.clusterRole(),
		c.clusterRoleBinding(),
		c.controllerDeployment(),
		c.controllerService(),
	}
	if c.rateLimitEnabled() {
		objs = append(objs, c.rateLimitDeployment(), c.rateLimitService())
	}
	// Render emits the upstream aigateway.envoyproxy.io CRDs unconditionally.
	// The controller (Phase 6) decides whether to reconcile them based on
	// AIGateway.Spec.CRDManagement.
	objs = append(objs, crdsFromEmbeddedChart()...)
	return objs, nil
}

func (c *component) rateLimitEnabled() bool {
	rl := c.cfg.AIGateway.Spec.RateLimitDeployment
	return rl == nil || !rl.Disabled
}

func (c *component) serviceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ServiceAccount"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ControllerName,
			Namespace: ControllerNamespace,
		},
	}
}

// clusterRole returns the controller's cluster-scoped permissions.
//
// The upstream ai-gateway-helm chart (templates/) ships only the inference-pool
// ClusterRole bound to envoy-gateway's SA; it does not template the controller's
// own RBAC. The rules below mirror the verbs the controller exercises against
// its own CRDs and the Gateway API objects it reconciles. Adjust if upstream
// later publishes a canonical role.
func (c *component) clusterRole() *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterRole", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: ControllerName},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"aigateway.envoyproxy.io"},
				Resources: []string{
					"aigatewayroutes", "aiservicebackends", "backendsecuritypolicies",
					"gatewayconfigs", "mcproutes", "quotapolicies",
				},
				Verbs: []string{"get", "list", "watch", "update", "patch"},
			},
			{
				APIGroups: []string{"aigateway.envoyproxy.io"},
				Resources: []string{"aigatewayroutes/status", "aiservicebackends/status"},
				Verbs:     []string{"get", "update", "patch"},
			},
			{
				APIGroups: []string{"gateway.networking.k8s.io"},
				Resources: []string{"gateways", "httproutes"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"secrets", "configmaps", "services"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch"},
			},
			{
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch"},
			},
		},
	}
}

func (c *component) clusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta:   metav1.TypeMeta{Kind: "ClusterRoleBinding", APIVersion: "rbac.authorization.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: ControllerName},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     ControllerName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      ControllerName,
			Namespace: ControllerNamespace,
		}},
	}
}

func (c *component) controllerDeployment() *appsv1.Deployment {
	labels := map[string]string{"app": ControllerName}
	podSpec := corev1.PodSpec{
		ServiceAccountName: ControllerName,
		Containers: []corev1.Container{{
			Name:  ControllerName,
			Image: c.controllerImage,
			Ports: []corev1.ContainerPort{{
				Name:          "extsrv",
				ContainerPort: ExtensionServerPort,
				Protocol:      corev1.ProtocolTCP,
			}},
		}},
	}

	// Apply customizations from the CR spec.
	var replicas *int32
	if d := c.cfg.AIGateway.Spec.AIGatewayControllerDeployment; d != nil && d.Spec != nil {
		replicas = d.Spec.Replicas
	}
	if tmpl := tmplSpecOrNil(c.cfg); tmpl != nil {
		if tmpl.Affinity != nil {
			podSpec.Affinity = tmpl.Affinity
		}
		if tmpl.NodeSelector != nil {
			podSpec.NodeSelector = tmpl.NodeSelector
		}
		if len(tmpl.Tolerations) > 0 {
			podSpec.Tolerations = tmpl.Tolerations
		}
		if len(tmpl.TopologySpreadConstraints) > 0 {
			podSpec.TopologySpreadConstraints = tmpl.TopologySpreadConstraints
		}
	}

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ControllerName,
			Namespace: ControllerNamespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       podSpec,
			},
		},
	}
}

// tmplSpecOrNil extracts the controller pod-template overrides from the CR,
// returning nil if any layer is unset.
func tmplSpecOrNil(cfg *Config) *operatorv1.AIGatewayControllerDeploymentPodSpec {
	d := cfg.AIGateway.Spec.AIGatewayControllerDeployment
	if d == nil || d.Spec == nil || d.Spec.Template == nil {
		return nil
	}
	return d.Spec.Template.Spec
}

func (c *component) controllerService() *corev1.Service {
	labels := map[string]string{"app": ControllerName}
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ControllerName,
			Namespace: ControllerNamespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:       "extsrv",
				Port:       ExtensionServerPort,
				TargetPort: intstr.FromInt(ExtensionServerPort),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}

func (c *component) rateLimitDeployment() *appsv1.Deployment {
	labels := map[string]string{"app": rateLimitName}
	// Temporarily reuses controllerImage; upstream may pack ratelimit alongside
	// controller. Phase 4C will swap to a dedicated image if needed.
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      rateLimitName,
			Namespace: ControllerNamespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  rateLimitContainerName,
						Image: c.controllerImage,
						Ports: []corev1.ContainerPort{{
							Name:          "grpc",
							ContainerPort: RateLimitPort,
							Protocol:      corev1.ProtocolTCP,
						}},
					}},
				},
			},
		},
	}
}

func (c *component) rateLimitService() *corev1.Service {
	labels := map[string]string{"app": rateLimitName}
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      rateLimitName,
			Namespace: ControllerNamespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:       "grpc",
				Port:       RateLimitPort,
				TargetPort: intstr.FromInt(RateLimitPort),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}

// crdsFromEmbeddedChart renders the embedded ai-gateway-crds-helm chart and
// returns the CustomResourceDefinition objects it produces. Cached behind a
// sync.Once because the chart inputs are static.
//
// TODO: dedupe with pkg/render/gatewayapi helm helper (renderChart / parseManifest).
var (
	crdsOnce    sync.Once
	crdsCached  []client.Object
	crdsErrored error
)

func crdsFromEmbeddedChart() []client.Object {
	crdsOnce.Do(func() {
		crdsCached, crdsErrored = renderCRDsChart()
	})
	if crdsErrored != nil {
		// The chart bytes are embedded at build time and have no value
		// overrides — failure here is a programmer error, not a runtime
		// condition. Panic so it surfaces in tests rather than emitting
		// silently empty CRDs.
		panic(fmt.Errorf("ai-gateway-crds chart render failed: %w", crdsErrored))
	}
	return crdsCached
}

func renderCRDsChart() ([]client.Object, error) {
	chart, err := loader.LoadArchive(bytes.NewReader(crdsChartBytes))
	if err != nil {
		return nil, fmt.Errorf("load ai-gateway-crds chart: %w", err)
	}

	helmClient := action.NewInstall(new(action.Configuration))
	helmClient.DryRun = true
	helmClient.ClientOnly = true
	helmClient.IncludeCRDs = true
	helmClient.Namespace = ControllerNamespace
	helmClient.ReleaseName = "tigera-ai-gateway-crds"

	rel, err := helmClient.Run(chart, nil)
	if err != nil {
		return nil, fmt.Errorf("render ai-gateway-crds chart: %w", err)
	}

	// Build a scheme with just apiextensions/v1 — the CRDs chart contains
	// nothing else, but we filter defensively below.
	scheme := runtime.NewScheme()
	if err := apiextv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register apiextensions/v1: %w", err)
	}
	codecs := serializer.NewCodecFactory(scheme)
	deserializer := codecs.UniversalDeserializer()

	var manifest bytes.Buffer
	manifest.WriteString(rel.Manifest)
	for _, hook := range rel.Hooks {
		manifest.WriteString("\n---\n")
		manifest.WriteString(hook.Manifest)
	}

	var out []client.Object
	decoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifest.Bytes()), 4096)
	for {
		var raw runtime.RawExtension
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode manifest doc: %w", err)
		}
		if len(raw.Raw) == 0 {
			continue
		}
		obj, _, err := deserializer.Decode(raw.Raw, nil, nil)
		if err != nil {
			// Skip docs the scheme can't decode (e.g. unexpected non-CRD docs).
			continue
		}
		if crd, ok := obj.(*apiextv1.CustomResourceDefinition); ok {
			out = append(out, crd)
		}
	}
	return out, nil
}
