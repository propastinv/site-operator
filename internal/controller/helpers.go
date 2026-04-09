package controller

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"

	sitev1alpha1 "github.com/propastinv/site-operator/api/v1alpha1"
)

func buildWPEnvs(site sitev1alpha1.Site) []corev1.EnvVar {
	scheme := "http"
	if site.Spec.Ingress != nil && site.Spec.Ingress.TLS != nil {
		if *site.Spec.Ingress.TLS {
			scheme = "https"
		}
	}

	wpDebug := false
	wpDebugLog := false
	wpDebugDisplay := false

	if site.Spec.Wordpress != nil && site.Spec.Wordpress.Debug != nil {
		wpDebug = site.Spec.Wordpress.Debug.Enabled
		wpDebugLog = site.Spec.Wordpress.Debug.Log
		wpDebugDisplay = site.Spec.Wordpress.Debug.Display
	}

	envs := []corev1.EnvVar{
		{
			Name:  "WP_HOME",
			Value: scheme + "://" + site.Spec.Domain,
		},
		{
			Name:  "WP_SITEURL",
			Value: scheme + "://" + site.Spec.Domain,
		},
		{
			Name:  "WP_DEBUG",
			Value: strconv.FormatBool(wpDebug),
		},
		{
			Name:  "WP_DEBUG_LOG",
			Value: strconv.FormatBool(wpDebugLog),
		},
		{
			Name:  "WP_DEBUG_DISPLAY",
			Value: strconv.FormatBool(wpDebugDisplay),
		},
		secretEnv(site, "AUTH_KEY"),
		secretEnv(site, "SECURE_AUTH_KEY"),
		secretEnv(site, "LOGGED_IN_KEY"),
		secretEnv(site, "NONCE_KEY"),
		secretEnv(site, "AUTH_SALT"),
		secretEnv(site, "SECURE_AUTH_SALT"),
		secretEnv(site, "LOGGED_IN_SALT"),
		secretEnv(site, "NONCE_SALT"),
	}

	// Add Installation Envs
	if site.Spec.Wordpress != nil && site.Spec.Wordpress.Install != nil {
		install := site.Spec.Wordpress.Install
		envs = append(envs, corev1.EnvVar{
			Name:  "WP_INSTALL",
			Value: "true",
		})
		envs = append(envs, corev1.EnvVar{
			Name:  "WP_ADMIN_USER",
			Value: install.AdminUser,
		})
		envs = append(envs, corev1.EnvVar{
			Name:  "WP_ADMIN_EMAIL",
			Value: install.AdminEmail,
		})

		title := install.Title
		if title == "" {
			title = site.Name
		}
		envs = append(envs, corev1.EnvVar{
			Name:  "WP_TITLE",
			Value: title,
		})

		// Admin Password
		if install.AdminPasswordSecret != nil {
			envs = append(envs, corev1.EnvVar{
				Name: "WP_ADMIN_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: install.AdminPasswordSecret.Name,
						},
						Key: install.AdminPasswordSecret.Key,
					},
				},
			})
		} else if install.AdminPassword != nil {
			envs = append(envs, corev1.EnvVar{
				Name:  "WP_ADMIN_PASSWORD",
				Value: *install.AdminPassword,
			})
		} else {
			// Default to generated password in our secret
			envs = append(envs, secretEnv(site, "ADMIN_PASSWORD"))
		}
	}

	return envs
}

func buildDatabaseEnvs(site sitev1alpha1.Site) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "DB_HOST",
			Value: site.Spec.Database.Host,
		},
		{
			Name:  "DB_NAME",
			Value: site.Spec.Database.Name,
		},
	}

	// The CRD enforces that exactly one credential method is used for each field.
	// We handle each field independently to support mixed configurations.

	// User
	if site.Spec.Database.UserSecret != nil {
		envs = append(envs, corev1.EnvVar{
			Name: "DB_USER",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: site.Spec.Database.UserSecret.Name,
					},
					Key: site.Spec.Database.UserSecret.Key,
				},
			},
		})
	} else if site.Spec.Database.User != nil {
		envs = append(envs, corev1.EnvVar{
			Name:  "DB_USER",
			Value: *site.Spec.Database.User,
		})
	}

	// Password
	if site.Spec.Database.PasswordSecret != nil {
		envs = append(envs, corev1.EnvVar{
			Name: "DB_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: site.Spec.Database.PasswordSecret.Name,
					},
					Key: site.Spec.Database.PasswordSecret.Key,
				},
			},
		})
	} else if site.Spec.Database.Password != nil {
		envs = append(envs, corev1.EnvVar{
			Name:  "DB_PASSWORD",
			Value: *site.Spec.Database.Password,
		})
	}

	return envs

}

func int32Ptr(i int32) *int32 {
	return &i
}

func int64Ptr(i int64) *int64 {
	return &i
}

func secretEnv(site sitev1alpha1.Site, name string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: site.Name + "-site-secret",
				},
				Key: name,
			},
		},
	}
}

func boolPtrVal(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
