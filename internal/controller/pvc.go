package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	sitev1alpha1 "github.com/propastinv/site-operator/api/v1alpha1"
)

func reconcilePVC(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	site sitev1alpha1.Site,
) error {

	if site.Spec.Persistence == nil || !site.Spec.Persistence.Enabled {
		return deleteOwnedPVC(ctx, c, site)
	}

	if site.Spec.Persistence.ExistingClaim != "" {
		return deleteOwnedPVC(ctx, c, site)
	}

	size := site.Spec.Persistence.Size
	if size == "" {
		size = "1Gi"
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      site.Name + "-data",
				Namespace: site.Namespace,
			},
		}

		_, err := controllerutil.CreateOrUpdate(ctx, c, pvc, func() error {

			pvc.Labels = map[string]string{
				"app": site.Name,
			}

			if pvc.CreationTimestamp.IsZero() {
				pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				}
			}

			if pvc.Spec.Resources.Requests == nil {
				pvc.Spec.Resources.Requests = corev1.ResourceList{}
			}
			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse(size)

			if pvc.CreationTimestamp.IsZero() {
				if site.Spec.Persistence.StorageClassName != nil {
					pvc.Spec.StorageClassName = site.Spec.Persistence.StorageClassName
				}
			}

			return controllerutil.SetControllerReference(&site, pvc, scheme)
		})
		return err
	})
}

func deleteOwnedPVC(
	ctx context.Context,
	c client.Client,
	site sitev1alpha1.Site,
) error {

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      site.Name + "-data",
			Namespace: site.Namespace,
		},
	}

	if err := c.Get(ctx, client.ObjectKeyFromObject(pvc), pvc); err != nil {
		return client.IgnoreNotFound(err)
	}

	if !metav1.IsControlledBy(pvc, &site) {
		return nil
	}

	return client.IgnoreNotFound(c.Delete(ctx, pvc))
}
