// Copyright (c) 2026 Tigera, Inc. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

// Package aigateway is the operator reconciler for the AIGateway CR.
// See tigera/designs#60.
package aigateway

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorv1 "github.com/tigera/operator/api/v1"
	"github.com/tigera/operator/pkg/controller/options"
	"github.com/tigera/operator/pkg/controller/status"
	"github.com/tigera/operator/pkg/controller/utils"
	"github.com/tigera/operator/pkg/ctrlruntime"
	aigwrender "github.com/tigera/operator/pkg/render/aigateway"
)

const aigatewayName = "default"

var log = logf.Log.WithName("controller_aigateway")

// Reconciler reconciles the AIGateway singleton.
type Reconciler struct {
	client client.Client
	scheme *runtime.Scheme
	status status.StatusManager
}

// NewReconciler builds a Reconciler with no StatusManager attached. Production
// callers must call SetStatus or use the (forthcoming) Add wiring; tests may
// drive Reconcile directly without a status manager.
func NewReconciler(c client.Client, s *runtime.Scheme) *Reconciler {
	return &Reconciler{client: c, scheme: s}
}

// SetStatus attaches a StatusManager to the reconciler. Used by tests and the
// production Add wiring (Phase 8).
func (r *Reconciler) SetStatus(s status.StatusManager) { r.status = s }

// Add creates a new AIGateway Controller and adds it to the Manager. The Manager
// will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, opts options.ControllerOptions) error {
	r := &Reconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		status: status.New(mgr.GetClient(), "aigateway", opts.KubernetesVersion),
	}
	r.status.Run(opts.ShutdownContext)

	c, err := ctrlruntime.NewController("aigateway-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return fmt.Errorf("failed to create aigateway-controller: %w", err)
	}

	// Watch the primary resource AIGateway.
	if err = c.WatchObject(&operatorv1.AIGateway{}, &handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("aigateway-controller failed to watch primary resource: %w", err)
	}

	// Watch Installation; AIGateway gates on the Enterprise variant.
	if err = utils.AddInstallationWatch(c); err != nil {
		return fmt.Errorf("aigateway-controller failed to watch Installation resource: %w", err)
	}

	// Watch GatewayAPI; AIGateway requires a 'default' GatewayAPI CR to exist
	// and validates against its GatewayClasses.
	if err = c.WatchObject(&operatorv1.GatewayAPI{}, &handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("aigateway-controller failed to watch GatewayAPI resource: %w", err)
	}

	// Periodic backstop reconcile to catch drift not caught by watches. This is
	// in addition to the RequeueAfter set by Reconcile on the happy path.
	if err = utils.AddPeriodicReconcile(c, utils.PeriodicReconcileTime, &handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("aigateway-controller failed to create periodic reconcile watch: %w", err)
	}

	return nil
}

// Reconcile loads the AIGateway CR and validates preconditions. Rendering
// and apply is implemented in a follow-up phase (6B / 6.5).
func (r *Reconciler) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	cr := &operatorv1.AIGateway{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: aigatewayName}, cr); err != nil {
		if apierrors.IsNotFound(err) {
			if r.status != nil {
				r.status.OnCRNotFound()
			}
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if r.status != nil {
		r.status.OnCRFound()
	}

	// Deletion: if the AIGateway CR is being deleted, drop the finalizer from
	// the controller Deployment so owner-ref GC can proceed and return.
	if cr.DeletionTimestamp != nil {
		d := &appsv1.Deployment{}
		if err := r.client.Get(ctx, types.NamespacedName{
			Name:      aigwrender.ControllerName,
			Namespace: aigwrender.ControllerNamespace,
		}, d); err == nil {
			if controllerutil.RemoveFinalizer(d, aigwrender.FinalizerName) {
				if err := r.client.Update(ctx, d); err != nil {
					return reconcile.Result{}, err
				}
			}
		}
		return reconcile.Result{}, nil
	}

	// Variant gate: AIGateway is an Enterprise-only feature.
	inst := &operatorv1.Installation{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: "default"}, inst); err != nil {
		if apierrors.IsNotFound(err) {
			if r.status != nil {
				r.status.SetDegraded(operatorv1.ResourceNotFound, "Installation not found", nil, log)
			}
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if inst.Spec.Variant != operatorv1.TigeraSecureEnterprise {
		if r.status != nil {
			r.status.SetDegraded(operatorv1.ResourceValidationError,
				"AIGateway requires Enterprise variant", nil, log)
		}
		return reconcile.Result{}, nil
	}

	// GatewayAPI presence: AIGateway extends the Envoy Gateway integration
	// that GatewayAPI provisions, so a 'default' GatewayAPI is required.
	gw := &operatorv1.GatewayAPI{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: "default"}, gw); err != nil {
		if apierrors.IsNotFound(err) {
			if r.status != nil {
				r.status.SetDegraded(operatorv1.ResourceNotReady,
					"GatewayAPI 'default' not found; AIGateway requires it", nil, log)
			}
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Cross-CR validation: every class referenced by AIGateway.Spec.GatewayClasses
	// MUST exist in GatewayAPI.Spec.GatewayClasses.
	known := make(map[string]struct{}, len(gw.Spec.GatewayClasses))
	for _, gc := range gw.Spec.GatewayClasses {
		known[gc.Name] = struct{}{}
	}
	for _, want := range cr.Spec.GatewayClasses {
		if _, ok := known[want]; !ok {
			if r.status != nil {
				r.status.SetDegraded(operatorv1.ResourceValidationError,
					fmt.Sprintf("AIGateway.Spec.GatewayClasses references unknown class %q", want),
					nil, log)
			}
			return reconcile.Result{}, nil
		}
	}

	// All validation gates passed. Render and apply the AI Gateway component.
	component := aigwrender.NewComponent(&aigwrender.Config{
		Installation: &inst.Spec,
		AIGateway:    cr,
	})
	if err := component.ResolveImages(nil); err != nil {
		if r.status != nil {
			r.status.SetDegraded(operatorv1.ResourceCreateError, "resolving images", err, log)
		}
		return reconcile.Result{}, err
	}
	handler := utils.NewComponentHandler(log, r.client, r.scheme, cr)
	if err := handler.CreateOrUpdateOrDelete(ctx, component, r.status); err != nil {
		if r.status != nil {
			r.status.SetDegraded(operatorv1.ResourceUpdateError, "applying AI Gateway component", err, log)
		}
		return reconcile.Result{}, err
	}
	if r.status != nil {
		r.status.ClearDegraded()
	}

	// Add a finalizer to the controller Deployment if not already present so
	// the operator can perform an orderly tear-down before owner-ref GC runs.
	d := &appsv1.Deployment{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      aigwrender.ControllerName,
		Namespace: aigwrender.ControllerNamespace,
	}, d); err == nil {
		if !controllerutil.ContainsFinalizer(d, aigwrender.FinalizerName) {
			controllerutil.AddFinalizer(d, aigwrender.FinalizerName)
			if err := r.client.Update(ctx, d); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	// Backstop reconcile so transient cluster drift (e.g. an out-of-band edit
	// to a managed object that doesn't fire a watch on this controller) is
	// eventually reconciled even without an explicit event.
	return reconcile.Result{RequeueAfter: 10 * time.Minute}, nil
}
