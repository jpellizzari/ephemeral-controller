/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ephemeralappsv1alpha1 "github.com/Hinge/ephemeral-controller/api/v1alpha1"
)

// EphemeralApplicationSetReconciler reconciles a EphemeralApplicationSet object
type EphemeralApplicationSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ephemeralapps.toolkit.hinge.com,resources=ephemeralapplicationsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ephemeralapps.toolkit.hinge.com,resources=ephemeralapplicationsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ephemeralapps.toolkit.hinge.com,resources=ephemeralapplicationsets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the EphemeralApplicationSet object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *EphemeralApplicationSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	as := &ephemeralappsv1alpha1.EphemeralApplicationSet{}

	if err := r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, as); err != nil {
		return ctrl.Result{}, fmt.Errorf("getting object: %w", err)
	}

	ephemeralNamespace := fmt.Sprintf("ephemeral-%s", as.Spec.FeatureTag)

	ns := &corev1.Namespace{}

	ns.Name = ephemeralNamespace

	if err := r.Client.Create(ctx, ns); err != nil {
		return ctrl.Result{}, fmt.Errorf("error creating namespace: %w", err)
	}

	isBeingDeleted := !as.DeletionTimestamp.IsZero()
	if isBeingDeleted {
		// Imagine doing our delete logic here.
		if controllerutil.ContainsFinalizer(as, "custom-finalizer.hinge.com") {
			// We can do pre-delete hooks here, like destroying AWS resources.
		}
	}

	for _, s := range as.Spec.Services {
		eph := &appsv1.Deployment{}
		eph.Name = s.Name
		eph.Namespace = ns.Name

		// See if an ephemeral app already exists
		existing := &appsv1.Deployment{}
		if err := r.Client.Get(ctx, types.NamespacedName{Name: eph.Name, Namespace: eph.Namespace}, existing); err != nil {
			return ctrl.Result{}, fmt.Errorf("could not get existing app: %w", err)
		}

		// Psuedo-code to demonstrate idemopotency; IRL we would update the exsiting with any new changes.
		// Pretend we are doing an update to `existing` here
		// ...

		// go get the original that is running in the cluster
		original := &appsv1.Deployment{}
		if err := r.Client.Get(ctx, types.NamespacedName{Name: s.Name, Namespace: "hinge-services"}, original); err != nil {
			return ctrl.Result{}, fmt.Errorf("finding original in hinge-services: %w", err)
		}

		eph.Spec = original.Spec
		// Assuming we only have one container for now
		container := eph.Spec.Template.Spec.Containers[0]
		container.Image = s.ImageTag
		container.Env = []corev1.EnvVar{}

		// Set up env vars
		for _, hs := range as.Spec.Services {
			if hs.Name == eph.Name {
				continue
			}

			// This would have to be a lookup rather than append to avoid dupes, but something like this.
			e := corev1.EnvVar{
				Name:  fmt.Sprintf("GRPC_CLIENT_TARGET_%s", hs.Name),
				Value: fmt.Sprintf("%s.%s.svc.cluster.local:50051", ns.Name),
			}

			container.Env = append(container.Env, e)

		}

		// Big Hand Wave
		// Now we would go read helm values from the repo and combine them with these local ones.
		// Maybe we can read these off the argo app? Right off the original deployment?
		// There is no reason why this controller can't clone the backend app and read values there. (besides size?)
		// Gitops-toolkit already supports this out of the box and can provide easily readable artifacts.
		var magic MagicValuesCombiner
		magic.CombineValues(s.Name, container.Env)

		// Add the ephemeral app to the cluster.
		// Nit: assuming .Create() here. Could be a .Update()
		if err := r.Client.Create(ctx, eph); err != nil {
			return ctrl.Result{}, fmt.Errorf("adding ephemeral app to cluster: %w", err)
		}
	}

	// Set up networking stuff?

	// Spin up a gateway deployment to route?

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EphemeralApplicationSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ephemeralappsv1alpha1.EphemeralApplicationSet{}).
		Named("ephemeralapplicationset").
		Complete(r)
}

type MagicValuesCombiner interface {
	CombineValues(name string, vars []corev1.EnvVar)
}
