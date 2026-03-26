/*
Copyright 2026.

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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sitev1alpha1 "github.com/propastinv/site-operator/api/v1alpha1"
)

// SiteReconciler reconciles a Site object
type SiteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// RBAC
// +kubebuilder:rbac:groups=site.propastinv,resources=sites,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=site.propastinv,resources=sites/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=site.propastinv,resources=sites/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;configmaps;persistentvolumeclaims;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Site object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/reconcile
func (r *SiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var site sitev1alpha1.Site
	if err := r.Get(ctx, req.NamespacedName, &site); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	labels := map[string]string{
		"app": site.Name,
	}

	// Secret (Salts and DB Password)
	if err := reconcileSecret(ctx, r.Client, r.Scheme, &site); err != nil {
		return ctrl.Result{}, err
	}

	envs := append(
		buildWPEnvs(site),
		buildDatabaseEnvs(site)...,
	)

	// Deployment
	if err := reconcileDeployment(ctx, r.Client, r.Scheme, site, labels, envs); err != nil {
		return ctrl.Result{}, err
	}

	// Service
	if err := reconcileService(ctx, r.Client, r.Scheme, site, labels); err != nil {
		return ctrl.Result{}, err
	}

	// Ingress
	if err := reconcileIngress(ctx, r.Client, r.Scheme, site, labels); err != nil {
		return ctrl.Result{}, err
	}

	// PVC
	if err := reconcilePVC(ctx, r.Client, r.Scheme, site); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&sitev1alpha1.Site{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Secret{}).
		Named("site").
		Complete(r)
}
