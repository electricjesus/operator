// Copyright (c) 2026 Tigera, Inc. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default'",message="resource name must be 'default'"

// AIGateway enables Envoy AI Gateway features on top of the Envoy Gateway
// integration provisioned via the GatewayAPI CR. Cluster-scoped singleton;
// name MUST be 'default'. Readiness reported via TigeraStatus 'aigateway'.
type AIGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AIGatewaySpec `json:"spec,omitempty"`
}

type AIGatewaySpec struct {
	// +optional
	GatewayClasses []string `json:"gatewayClasses,omitempty"`
	// +optional
	AIGatewayControllerDeployment *AIGatewayControllerDeployment `json:"aiGatewayControllerDeployment,omitempty"`
	// +optional
	ExtProcContainer *ExtProcContainer `json:"extProcContainer,omitempty"`
	// +optional
	RateLimitDeployment *RateLimitDeployment `json:"rateLimitDeployment,omitempty"`
	// +optional
	CRDManagement *CRDManagement `json:"crdManagement,omitempty"`
}

type AIGatewayControllerDeployment struct {
	// +optional
	Spec *AIGatewayControllerDeploymentSpec `json:"spec,omitempty"`
}

type AIGatewayControllerDeploymentSpec struct {
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`
	// +optional
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`
	// +optional
	Template *AIGatewayControllerDeploymentPodTemplateSpec `json:"template,omitempty"`
}

type AIGatewayControllerDeploymentPodTemplateSpec struct {
	// +optional
	Spec *AIGatewayControllerDeploymentPodSpec `json:"spec,omitempty"`
}

type AIGatewayControllerDeploymentPodSpec struct {
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// +optional
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	// +optional
	Containers []AIGatewayControllerContainer `json:"containers,omitempty"`
}

type AIGatewayControllerContainer struct {
	// +kubebuilder:validation:Enum=envoy-ai-gateway-controller
	Name string `json:"name"`
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// +optional
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`
	// +optional
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`
}

type ExtProcContainer struct {
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
	// +optional
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`
	// +optional
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`
}

type RateLimitDeployment struct {
	// +optional
	Disabled bool `json:"disabled,omitempty"`
	// +optional
	Spec *RateLimitDeploymentSpec `json:"spec,omitempty"`
}

type RateLimitDeploymentSpec struct {
	// +optional
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`
	// +optional
	Template *AIGatewayControllerDeploymentPodTemplateSpec `json:"template,omitempty"`
}

// +kubebuilder:object:root=true

type AIGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIGateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIGateway{}, &AIGatewayList{})
}
