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
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sitev1alpha1 "github.com/propastinv/site-operator/api/v1alpha1"
)

// SiteReconciler reconciles a Site object
type SiteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// DefaultMariaDBRef, when set, is used for spec.provision.database on any
	// Site that doesn't specify its own mariadbRef. Configured via the
	// --default-mariadb-name/--default-mariadb-namespace flags, exposed as
	// provision.database.mariadbRef in the site-operator Helm chart.
	DefaultMariaDBRef *sitev1alpha1.MariaDBClusterRef
	// DefaultStorageClassName, when set, is used for spec.persistence on any
	// Site that doesn't specify its own storageClassName. Configured via the
	// --default-storage-class-name flag, exposed as persistence.storageClassName
	// in the site-operator Helm chart.
	DefaultStorageClassName string
}

// RBAC
// +kubebuilder:rbac:groups=site.operator,resources=sites,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=site.operator,resources=sites/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=site.operator,resources=sites/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;configmaps;persistentvolumeclaims;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=k8s.mariadb.com,resources=databases;users;grants,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Site object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/reconcile
func (r *SiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, reconcileErr error) {
	var site sitev1alpha1.Site
	if err := r.Get(ctx, req.NamespacedName, &site); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	defer func() {
		if statusErr := r.updateSiteStatus(ctx, &site, reconcileErr); statusErr != nil && reconcileErr == nil {
			reconcileErr = statusErr
		}
	}()

	labels := map[string]string{
		"app": site.Name,
	}

	// Secret (Salts and DB Password)
	if err := reconcileSecret(ctx, r.Client, r.Scheme, &site, &site); err != nil {
		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}

	// Database provisioning (optional, via mariadb-operator)
	if err := reconcileDatabaseProvision(ctx, r.Client, r.Scheme, &site, site, r.DefaultMariaDBRef); err != nil {
		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}

	envs := append(
		buildWPEnvs(site),
		buildDatabaseEnvs(site)...,
	)

	// Deployment
	if err := reconcileDeployment(ctx, r.Client, r.Scheme, &site, site, labels, envs); err != nil {
		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}

	// Service
	if err := reconcileService(ctx, r.Client, r.Scheme, &site, site, labels); err != nil {
		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}

	// Ingress
	if err := reconcileIngress(ctx, r.Client, r.Scheme, &site, site, labels); err != nil {
		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}

	// PVC
	if err := reconcilePVC(ctx, r.Client, r.Scheme, &site, site, r.DefaultStorageClassName); err != nil {
		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}

	return ctrl.Result{}, nil
}

// updateSiteStatus refreshes the Site's Available/Progressing/Degraded conditions
// based on the outcome of this reconcile and the state of its owned Deployment.
// It re-fetches the Site on each conflict retry so it always patches the latest
// resourceVersion instead of clobbering a concurrent status write.
func (r *SiteReconciler) updateSiteStatus(ctx context.Context, site *sitev1alpha1.Site, reconcileErr error) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &sitev1alpha1.Site{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(site), current); err != nil {
			return client.IgnoreNotFound(err)
		}

		var deploy appsv1.Deployment
		deployErr := r.Get(ctx, client.ObjectKeyFromObject(current), &deploy)

		setSiteConditions(&current.Status.Conditions, current.Generation, &deploy, deployErr, reconcileErr)

		return r.Status().Update(ctx, current)
	})
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
