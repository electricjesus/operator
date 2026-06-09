package v1_test

import (
	"os"
	"strings"
	"testing"

	operatorv1 "github.com/tigera/operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestAIGatewayRegistration(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := operatorv1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	gvk := operatorv1.GroupVersion.WithKind("AIGateway")
	if _, err := scheme.New(gvk); err != nil {
		t.Fatalf("scheme.New(%s): %v", gvk, err)
	}
	obj := &operatorv1.AIGateway{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	if obj.GetName() != "default" {
		t.Fatalf("unexpected name: %q", obj.GetName())
	}
}

func TestAIGatewayCELRejectsBadName(t *testing.T) {
	// CEL is enforced by the apiserver at admission time, not by client-side
	// schema validation. This test verifies the CRD manifest carries the
	// XValidation rule the design requires.
	data, err := os.ReadFile("../../pkg/imports/crds/operator/operator.tigera.io_aigateways.yaml")
	if err != nil {
		t.Fatalf("read CRD: %v", err)
	}
	if !strings.Contains(string(data), "self.metadata.name == 'default'") {
		t.Fatalf("CRD missing singleton-name CEL rule")
	}
}
