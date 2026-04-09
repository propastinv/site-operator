package controller

import (
	"context"
	"encoding/json"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	legacyv1alpha1 "github.com/propastinv/site-operator/api/legacy/v1alpha1"
	sitev1alpha1 "github.com/propastinv/site-operator/api/v1alpha1"
)

// LegacySiteReconciler reconciles legacy Site objects from the site.propastinv group.
//
// RBAC
// +kubebuilder:rbac:groups=site.propastinv,resources=sites,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=site.propastinv,resources=sites/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=site.propastinv,resources=sites/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;configmaps;persistentvolumeclaims;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
type LegacySiteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *LegacySiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var legacySite legacyv1alpha1.Site
	if err := r.Get(ctx, req.NamespacedName, &legacySite); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	site, err := convertLegacySiteToNew(&legacySite)
	if err != nil {
		return ctrl.Result{}, err
	}

	labels := map[string]string{
		"app": site.Name,
	}

	if err := reconcileSecret(ctx, r.Client, r.Scheme, &legacySite, &site); err != nil {
		return ctrl.Result{}, err
	}

	envs := append(
		buildWPEnvs(site),
		buildDatabaseEnvs(site)...,
	)

	if err := reconcileDeployment(ctx, r.Client, r.Scheme, &legacySite, site, labels, envs); err != nil {
		return ctrl.Result{}, err
	}

	if err := reconcileService(ctx, r.Client, r.Scheme, &legacySite, site, labels); err != nil {
		return ctrl.Result{}, err
	}

	if err := reconcileIngress(ctx, r.Client, r.Scheme, &legacySite, site, labels); err != nil {
		return ctrl.Result{}, err
	}

	if err := reconcilePVC(ctx, r.Client, r.Scheme, &legacySite, site); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *LegacySiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&legacyv1alpha1.Site{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Secret{}).
		Named("legacy-site").
		Complete(r)
}

func convertLegacySiteToNew(in *legacyv1alpha1.Site) (sitev1alpha1.Site, error) {
	// Specs are structurally identical; JSON round-trip keeps the conversion robust
	// and avoids keeping two copies of reconcile logic.
	b, err := json.Marshal(in)
	if err != nil {
		return sitev1alpha1.Site{}, err
	}

	var out sitev1alpha1.Site
	if err := json.Unmarshal(b, &out); err != nil {
		return sitev1alpha1.Site{}, err
	}

	return out, nil
}

