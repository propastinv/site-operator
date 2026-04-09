package controller

import (
	"context"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	sitev1alpha1 "github.com/propastinv/site-operator/api/v1alpha1"
)

func reconcileIngress(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	owner metav1.Object,
	site sitev1alpha1.Site,
	labels map[string]string,
) error {

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      site.Name + "-ingress",
			Namespace: site.Namespace,
		},
	}

	if site.Spec.Ingress == nil || !site.Spec.Ingress.Enabled {

		if err := c.Get(ctx, client.ObjectKeyFromObject(ing), ing); err != nil {
			return client.IgnoreNotFound(err)
		}

		if !metav1.IsControlledBy(ing, owner) {
			return nil
		}

		return client.IgnoreNotFound(c.Delete(ctx, ing))
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Re-initialize the object for the retry loop to ensure we have a clean state
		ing := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      site.Name + "-ingress",
				Namespace: site.Namespace,
			},
		}

		_, err := controllerutil.CreateOrUpdate(ctx, c, ing, func() error {

			ing.Labels = labels
			ing.Annotations = site.Spec.Ingress.Annotations
			if site.Spec.Ingress.IngressClassName != "" {
				ing.Spec.IngressClassName = &site.Spec.Ingress.IngressClassName
			}

			ing.Spec.Rules = []networkingv1.IngressRule{
				{
					Host: site.Spec.Domain,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     site.Spec.Ingress.Path,
									PathType: ptrPathType(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: site.Name,
											Port: networkingv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			}

			if site.Spec.Ingress.TLS != nil && *site.Spec.Ingress.TLS {
				ing.Spec.TLS = []networkingv1.IngressTLS{
					{
						Hosts:      []string{site.Spec.Domain},
						SecretName: site.Name + "-tls",
					},
				}
			} else {
				ing.Spec.TLS = nil
			}

			return controllerutil.SetControllerReference(owner, ing, scheme)
		})
		return err
	})
}

func ptrPathType(p networkingv1.PathType) *networkingv1.PathType {
	return &p
}
