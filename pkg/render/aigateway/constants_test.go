// Copyright (c) 2026 Tigera, Inc. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package aigateway_test

import (
	"testing"

	"github.com/tigera/operator/pkg/render/aigateway"
)

func TestConstants(t *testing.T) {
	cases := []struct{ got, want string }{
		{aigateway.ControllerName, "envoy-ai-gateway-controller"},
		{aigateway.ControllerNamespace, "calico-system"},
		{aigateway.ExtensionServerSvc, "envoy-ai-gateway-controller.calico-system.svc"},
		{aigateway.ExtProcUDSSocketPath, "unix:///var/run/extproc/ext_proc.sock"},
		{aigateway.ExtProcFilterConfigPath, "/etc/filter-config/filter-config.yaml"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("got %q want %q", tc.got, tc.want)
		}
	}
	if aigateway.ExtensionServerPort != 1063 {
		t.Errorf("ExtensionServerPort: got %d want 1063", aigateway.ExtensionServerPort)
	}
}
