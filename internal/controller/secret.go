package controller

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	sitev1alpha1 "github.com/propastinv/site-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func reconcileSecret(ctx context.Context, c client.Client, scheme *runtime.Scheme, owner metav1.Object, site *sitev1alpha1.Site) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      site.Name + "-site-secret",
			Namespace: site.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, c, secret, func() error {
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}

		// WordPress Salts
		salts := []string{
			"AUTH_KEY", "SECURE_AUTH_KEY", "LOGGED_IN_KEY", "NONCE_KEY",
			"AUTH_SALT", "SECURE_AUTH_SALT", "LOGGED_IN_SALT", "NONCE_SALT",
		}
		for _, s := range salts {
			if _, ok := secret.Data[s]; !ok {
				secret.Data[s] = []byte(randomKey())
			}
		}

		// Database Password (if automated provisioning is used)
		if site.Spec.Database.Host == "" {
			if _, ok := secret.Data["password"]; !ok {
				secret.Data["password"] = []byte(randomKey())
			}
		}

		// WordPress Admin Password
		if site.Spec.Wordpress != nil && site.Spec.Wordpress.Install != nil {
			install := site.Spec.Wordpress.Install
			if install.AdminPasswordSecret == nil && install.AdminPassword == nil {
				if _, ok := secret.Data["ADMIN_PASSWORD"]; !ok {
					secret.Data["ADMIN_PASSWORD"] = []byte(randomKey())
				}
			}
		}

		return controllerutil.SetControllerReference(owner, secret, scheme)
	})

	return err
}

func randomKey() string {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return "fallback-random-key-if-entropy-fails"
	}
	return base64.StdEncoding.EncodeToString(b)
}
