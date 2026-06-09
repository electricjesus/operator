// Copyright (c) 2026 Tigera, Inc. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package aigateway

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	operatorv1 "github.com/tigera/operator/api/v1"
)

// RecommendedExtensionHooks returns the upstream-recommended xdsTranslator
// post-hook set for the pinned EAIG version. Operator pins this set; users
// cannot override it.
func RecommendedExtensionHooks() []string {
	return []string{"VirtualHost", "Translation", "HTTPListener", "Route", "Cluster"}
}

// ExtProcContainer renders the ext_proc sidecar that the gatewayapi
// controller appends to per-class EnvoyProxy.Spec.Provider.Kubernetes.
// EnvoyDeployment.Pod.Containers. Image is operator-controlled; overrides
// affect resources, env, and probes only. The filter-config Secret it
// reads is written by the upstream AI Gateway controller and substituted
// per-Gateway at pod-creation time by its mutating webhook — the
// operator's EnvoyProxy template carries only a static volume name.
func ExtProcContainer(image string, overrides *operatorv1.ExtProcContainer) corev1.Container {
	c := corev1.Container{
		Name:  ExtProcContainerName,
		Image: image,
		Args: []string{
			"-configPath=" + ExtProcFilterConfigPath,
			"-extProcAddr=" + ExtProcUDSSocketPath,
		},
		// Kubernetes native sidecar restart policy. Envoy Gateway exposes
		// only InitContainers on its KubernetesPodSpec; using
		// `RestartPolicy=Always` on an init container makes it a sidecar
		// that runs alongside the envoy container.
		RestartPolicy: ptr.To(corev1.ContainerRestartPolicyAlways),
		VolumeMounts: []corev1.VolumeMount{
			{Name: ExtProcFilterConfigVol, MountPath: ExtProcFilterConfigDir, ReadOnly: true},
			{Name: ExtProcUDSVolumeName, MountPath: ExtProcUDSMountPath},
		},
	}
	if overrides != nil {
		if overrides.Resources != nil {
			c.Resources = *overrides.Resources
		}
		if len(overrides.Env) > 0 {
			c.Env = append(c.Env, overrides.Env...)
		}
		if overrides.LivenessProbe != nil {
			c.LivenessProbe = overrides.LivenessProbe
		}
		if overrides.ReadinessProbe != nil {
			c.ReadinessProbe = overrides.ReadinessProbe
		}
	}
	return c
}

// ExtProcVolumes returns the Volume entries the gatewayapi render adds to
// EnvoyProxy.Spec.Provider.Kubernetes.EnvoyDeployment.Pod.Volumes when AI
// is enabled. The filter-config Secret reference is rewritten per-Gateway
// at admission time by the upstream AI Gateway controller's mutating
// webhook; the EnvoyProxy template (which is per-class, not per-Gateway)
// carries only a placeholder Secret name that the webhook substitutes.
func ExtProcVolumes() []corev1.Volume {
	return []corev1.Volume{
		{
			Name: ExtProcFilterConfigVol,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: ExtProcFilterConfigVol},
			},
		},
		{
			Name:         ExtProcUDSVolumeName,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
	}
}
