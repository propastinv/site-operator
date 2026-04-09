package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	sitev1alpha1 "github.com/propastinv/site-operator/api/v1alpha1"
)

func reconcileService(ctx context.Context, c client.Client, scheme *runtime.Scheme, owner metav1.Object, site sitev1alpha1.Site, labels map[string]string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      site.Name,
				Namespace: site.Namespace,
			},
		}

		_, err := controllerutil.CreateOrUpdate(ctx, c, svc, func() error {
			svc.Labels = labels
			svc.Spec.Selector = labels
			svc.Spec.Ports = []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(80),
				},
			}
			return controllerutil.SetControllerReference(owner, svc, scheme)
		})
		return err
	})
}
