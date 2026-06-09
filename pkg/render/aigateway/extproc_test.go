// Copyright (c) 2026 Tigera, Inc. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package aigateway_test

import (
	"testing"

	operatorv1 "github.com/tigera/operator/api/v1"
	"github.com/tigera/operator/pkg/render/aigateway"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestRecommendedExtensionHooks(t *testing.T) {
	got := aigateway.RecommendedExtensionHooks()
	want := []string{"VirtualHost", "Translation", "HTTPListener", "Route", "Cluster"}
	if len(got) != len(want) {
		t.Fatalf("got %d hooks want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q want %q", i, got[i], want[i])
		}
	}
}

func TestExtProcContainer(t *testing.T) {
	got := aigateway.ExtProcContainer("nginx:fake", nil)
	if got.Name != aigateway.ExtProcContainerName {
		t.Errorf("name: got %q want %q", got.Name, aigateway.ExtProcContainerName)
	}
	if got.Image != "nginx:fake" {
		t.Errorf("image: got %q", got.Image)
	}
	wantArgs := map[string]bool{
		"-configPath=" + aigateway.ExtProcFilterConfigPath: false,
		"-extProcAddr=" + aigateway.ExtProcUDSSocketPath:   false,
	}
	for _, a := range got.Args {
		if _, ok := wantArgs[a]; ok {
			wantArgs[a] = true
		}
	}
	for k, v := range wantArgs {
		if !v {
			t.Errorf("missing arg %q", k)
		}
	}
	hasFilterMount := false
	hasUDSMount := false
	for _, m := range got.VolumeMounts {
		if m.Name == aigateway.ExtProcFilterConfigVol && m.MountPath == aigateway.ExtProcFilterConfigDir && m.ReadOnly {
			hasFilterMount = true
		}
		if m.Name == aigateway.ExtProcUDSVolumeName && m.MountPath == aigateway.ExtProcUDSMountPath {
			hasUDSMount = true
		}
	}
	if !hasFilterMount {
		t.Error("missing filter-config volume mount")
	}
	if !hasUDSMount {
		t.Error("missing UDS volume mount")
	}
}

func TestExtProcContainerOverrides(t *testing.T) {
	want := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("256Mi")},
	}
	got := aigateway.ExtProcContainer("nginx:fake", &operatorv1.ExtProcContainer{
		Resources: &want,
		Env:       []corev1.EnvVar{{Name: "LOG_LEVEL", Value: "debug"}},
	})
	if got.Resources.Limits[corev1.ResourceMemory] != want.Limits[corev1.ResourceMemory] {
		t.Errorf("resources not applied: %+v", got.Resources)
	}
	foundEnv := false
	for _, e := range got.Env {
		if e.Name == "LOG_LEVEL" && e.Value == "debug" {
			foundEnv = true
		}
	}
	if !foundEnv {
		t.Error("env override not applied")
	}
}

func TestExtProcVolumes(t *testing.T) {
	vols := aigateway.ExtProcVolumes()
	if len(vols) != 2 {
		t.Fatalf("got %d volumes want 2", len(vols))
	}
	var sec, uds *corev1.Volume
	for i, v := range vols {
		switch v.Name {
		case aigateway.ExtProcFilterConfigVol:
			sec = &vols[i]
		case aigateway.ExtProcUDSVolumeName:
			uds = &vols[i]
		}
	}
	if sec == nil || sec.Secret == nil || sec.Secret.SecretName != aigateway.ExtProcFilterConfigVol {
		t.Errorf("filter-config volume: %+v", sec)
	}
	if uds == nil || uds.EmptyDir == nil {
		t.Errorf("uds volume: %+v", uds)
	}
}
