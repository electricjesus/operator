// Copyright (c) 2026 Tigera, Inc. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

// Package aigateway renders Envoy AI Gateway resources and exports the
// constants the gatewayapi package needs to wire its extensionManager
// overlay and ext_proc sidecar render. See tigera/designs#60.
package aigateway

const (
	ControllerName      = "envoy-ai-gateway-controller"
	ControllerNamespace = "calico-system"
	ExtensionServerPort = 1063
	ExtensionServerSvc  = "envoy-ai-gateway-controller.calico-system.svc"

	ExtProcContainerName    = "ai-gateway-extproc"
	ExtProcUDSVolumeName    = "extproc-uds"
	ExtProcUDSMountPath     = "/var/run/extproc"
	ExtProcUDSSocketPath    = "unix:///var/run/extproc/ext_proc.sock"
	ExtProcFilterConfigVol  = "filter-config"
	ExtProcFilterConfigPath = "/etc/filter-config/filter-config.yaml"
	ExtProcFilterConfigKey  = "filter-config.yaml"
	ExtProcFilterConfigDir  = "/etc/filter-config"

	RateLimitSvc  = "envoy-ai-gateway-ratelimit.calico-system.svc"
	RateLimitPort = 4317

	// HACK(hackathon): pin EAIG controller/extproc to upstream vendor images;
	// revert to Tigera-mirrored + normal registry composition post-hackathon.
	// EAIG images are NOT mirrored in the Tigera registry, so the normal
	// component/ResolveImages path resolves to an unpullable
	// <registry>/<imagePath>/envoy-ai-gateway-*:v0.7.0 ref and the demo
	// deploys into ImagePullBackOff. Pin the literal upstream refs instead.
	UpstreamControllerImage = "docker.io/envoyproxy/ai-gateway-controller:v0.7.0"
	UpstreamExtProcImage    = "docker.io/envoyproxy/ai-gateway-extproc:v0.7.0"

	TigeraStatusName = "aigateway"
	FinalizerName    = "operator.tigera.io/aigateway-controller"
)
