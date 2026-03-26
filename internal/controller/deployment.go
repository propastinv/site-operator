package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	sitev1alpha1 "github.com/propastinv/site-operator/api/v1alpha1"
)

func reconcileDeployment(ctx context.Context, c client.Client, scheme *runtime.Scheme, site sitev1alpha1.Site, labels map[string]string, envs []corev1.EnvVar) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		nginxConfig := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      site.Name + "-nginx",
				Namespace: site.Namespace,
			},
		}

		_, err := controllerutil.CreateOrUpdate(ctx, c, nginxConfig, func() error {
			tlsEnabled := site.Spec.Ingress != nil && boolPtrVal(site.Spec.Ingress.TLS)

			fastcgiHTTPS := ""
			fastcgiXFP := ""
			if tlsEnabled {
				fastcgiHTTPS = "fastcgi_param HTTPS on;"
				fastcgiXFP = "fastcgi_param HTTP_X_FORWARDED_PROTO $scheme;"
			}

			nginxConfig.Data = map[string]string{
				"default.conf": fmt.Sprintf(`
server {
  listen 80;
  server_name _;

  root /var/www/html;
  index index.php index.html;

  location / {
    try_files $uri $uri/ /index.php?$args;
  }

  location ~ \.php$ {
    include fastcgi_params;
    fastcgi_pass 127.0.0.1:9000;
    fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;

    %s
    %s
  }
}
`, fastcgiHTTPS, fastcgiXFP),
			}

			return controllerutil.SetControllerReference(&site, nginxConfig, scheme)
		})
		return err
	})
	if err != nil {
		return err
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      site.Name,
				Namespace: site.Namespace,
			},
		}

		_, err := controllerutil.CreateOrUpdate(ctx, c, deploy, func() error {
			deploy.Labels = labels
			deploy.Spec = buildDeploymentSpec(site, labels, envs)
			return controllerutil.SetControllerReference(&site, deploy, scheme)
		})
		return err
	})
}

func buildDeploymentSpec(site sitev1alpha1.Site, labels map[string]string, envs []corev1.EnvVar) appsv1.DeploymentSpec {
	return appsv1.DeploymentSpec{
		Replicas: int32Ptr(1),
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
			},
			Spec: corev1.PodSpec{
				NodeSelector: site.Spec.NodeSelector,
				Volumes: []corev1.Volume{
					buildSiteDataVolume(site),
					{
						Name: "nginx-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: site.Name + "-nginx",
								},
							},
						},
					},
				},
				InitContainers: []corev1.Container{
					buildWPInitContainer(),
				},
				Containers: []corev1.Container{
					buildPHPFPMContainer(envs),
					buildNginxContainer(),
				},
			},
		},
	}
}

func buildWPInitContainer() corev1.Container {
	return corev1.Container{
		Name:    "wp-init",
		Image:   "wordpress:php8.5-fpm",
		Command: []string{"sh", "-c"},
		Args: []string{`
set -e

if [ ! -f /var/www/html/index.php ]; then
  echo "Initializing WordPress files..."
  cp -r /usr/src/wordpress/* /var/www/html/
  chown -R www-data:www-data /var/www/html
fi
`},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "site-data", MountPath: "/var/www/html"},
		},
	}
}

func buildPHPFPMContainer(envs []corev1.EnvVar) corev1.Container {
	return corev1.Container{
		Name:    "php-fpm",
		Image:   "wordpress:php8.5-fpm",
		Command: []string{"sh", "-c"},
		Args: []string{`
set -e

CONFIG=/var/www/html/wp-config.php

cat > $CONFIG <<EOF
<?php
define('DB_NAME', getenv('DB_NAME'));
define('DB_USER', getenv('DB_USER'));
define('DB_PASSWORD', getenv('DB_PASSWORD'));
define('DB_HOST', getenv('DB_HOST'));

define('WP_HOME', getenv('WP_HOME'));
define('WP_SITEURL', getenv('WP_SITEURL'));

if (getenv('WP_HOME') && str_starts_with(getenv('WP_HOME'), 'https://')) {
    \$_SERVER['HTTPS'] = 'on';
    \$_SERVER['SERVER_PORT'] = 443;
    if (!defined('FORCE_SSL_ADMIN')) {
        define('FORCE_SSL_ADMIN', true);
    }
}

define('AUTH_KEY', getenv('AUTH_KEY'));
define('SECURE_AUTH_KEY', getenv('SECURE_AUTH_KEY'));
define('LOGGED_IN_KEY', getenv('LOGGED_IN_KEY'));
define('NONCE_KEY', getenv('NONCE_KEY'));
define('AUTH_SALT', getenv('AUTH_SALT'));
define('SECURE_AUTH_SALT', getenv('SECURE_AUTH_SALT'));
define('LOGGED_IN_SALT', getenv('LOGGED_IN_SALT'));
define('NONCE_SALT', getenv('NONCE_SALT'));

\$table_prefix = 'wp_';

define('WP_DEBUG', getenv('WP_DEBUG') === 'true');
define('WP_DEBUG_LOG', getenv('WP_DEBUG_LOG') === 'true');
define('WP_DEBUG_DISPLAY', getenv('WP_DEBUG_DISPLAY') === 'true');

if ( ! defined( 'ABSPATH' ) ) {
	define( 'ABSPATH', __DIR__ . '/' );
}

require_once ABSPATH . 'wp-settings.php';
define( 'FS_METHOD', 'direct' );
EOF

exec php-fpm
`},
		Ports: []corev1.ContainerPort{
			{ContainerPort: 9000},
		},
		Env: envs,
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:  int64Ptr(33),
			RunAsGroup: int64Ptr(33),
		},

		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(9000),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
		},

		VolumeMounts: []corev1.VolumeMount{
			{Name: "site-data", MountPath: "/var/www/html"},
		},
	}
}

func buildNginxContainer() corev1.Container {
	return corev1.Container{
		Name:  "nginx",
		Image: "nginx:1.25-alpine",
		Ports: []corev1.ContainerPort{
			{ContainerPort: 80},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "site-data", MountPath: "/var/www/html"},
			{Name: "nginx-config", MountPath: "/etc/nginx/conf.d/default.conf", SubPath: "default.conf"},
		},
	}
}

func buildSiteDataVolume(site sitev1alpha1.Site) corev1.Volume {

	if site.Spec.Persistence == nil || !site.Spec.Persistence.Enabled {
		return corev1.Volume{
			Name: "site-data",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
	}

	claimName := site.Name + "-data"

	if site.Spec.Persistence.ExistingClaim != "" {
		claimName = site.Spec.Persistence.ExistingClaim
	}

	return corev1.Volume{
		Name: "site-data",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: claimName,
			},
		},
	}
}
