package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

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

// phpIniFileName is the name under which the generated php.ini overrides are
// stored both in the ConfigMap and mounted in php-fpm's conf.d. The "zz-"
// prefix makes it sort after the image's own conf.d/*.ini files, so ours win.
const phpIniFileName = "zz-site-operator.ini"

const (
	// siteDataVolume is the name of the volume holding the site's files, and
	// siteRoot is where it is mounted in every container.
	siteDataVolume = "site-data"
	siteRoot       = "/var/www/html"
)

// Images used when a Site does not set spec.php.image / spec.nginx.image.
// wp-init and php-fpm share the PHP image: wp-init copies WordPress from
// /usr/src/wordpress, which only exists in the official wordpress image.
const (
	defaultPHPImage   = "wordpress:php8.5-fpm"
	defaultNginxImage = "nginx:1.30-alpine"
)

func phpImage(site sitev1alpha1.Site) string {
	if site.Spec.Php != nil && site.Spec.Php.Image != "" {
		return site.Spec.Php.Image
	}
	return defaultPHPImage
}

func nginxImage(site sitev1alpha1.Site) string {
	if site.Spec.Nginx != nil && site.Spec.Nginx.Image != "" {
		return site.Spec.Nginx.Image
	}
	return defaultNginxImage
}

func reconcileDeployment(ctx context.Context, c client.Client, scheme *runtime.Scheme, owner metav1.Object, site sitev1alpha1.Site, labels map[string]string, envs []corev1.EnvVar) error {
	nginxContent := buildNginxConfigContent(site)
	phpIniContent := buildPHPIniContent(site)

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		nginxConfig := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      site.Name + "-nginx",
				Namespace: site.Namespace,
			},
		}

		_, err := controllerutil.CreateOrUpdate(ctx, c, nginxConfig, func() error {
			nginxConfig.Data = map[string]string{
				"default.conf": nginxContent,
			}
			return controllerutil.SetControllerReference(owner, nginxConfig, scheme)
		})
		return err
	})
	if err != nil {
		return err
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		phpConfig := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      site.Name + "-php",
				Namespace: site.Namespace,
			},
		}

		_, err := controllerutil.CreateOrUpdate(ctx, c, phpConfig, func() error {
			phpConfig.Data = map[string]string{
				phpIniFileName: phpIniContent,
			}
			return controllerutil.SetControllerReference(owner, phpConfig, scheme)
		})
		return err
	})
	if err != nil {
		return err
	}

	configHash := configChecksum(nginxContent, phpIniContent)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      site.Name,
				Namespace: site.Namespace,
			},
		}

		_, err := controllerutil.CreateOrUpdate(ctx, c, deploy, func() error {
			deploy.Labels = labels
			deploy.Spec = buildDeploymentSpec(site, labels, envs, configHash)
			return controllerutil.SetControllerReference(owner, deploy, scheme)
		})
		return err
	})
}

// buildNginxConfigContent renders the nginx server{} block, including any
// raw directives from spec.nginx.config.
func buildNginxConfigContent(site sitev1alpha1.Site) string {
	tlsEnabled := site.Spec.Ingress != nil && boolPtrVal(site.Spec.Ingress.TLS)

	fastcgiHTTPS := ""
	fastcgiXFP := ""
	if tlsEnabled {
		fastcgiHTTPS = "fastcgi_param HTTPS on;"
		// Literal, not $scheme: TLS ends at the ingress, so nginx's own $scheme is
		// always "http" and would tell PHP the opposite of what HTTPS on says.
		fastcgiXFP = "fastcgi_param HTTP_X_FORWARDED_PROTO https;"
	}

	extraConfig := ""
	if site.Spec.Nginx != nil {
		extraConfig = site.Spec.Nginx.Config
	}

	// TLS terminates at the ingress, so nginx only ever sees plain http. Its own
	// redirects (e.g. /dir -> /dir/ for a real directory under the web root) would
	// otherwise carry an absolute http:// Location and downgrade https visitors.
	// Relative Locations keep whatever scheme the client used. Skipped when the
	// site sets the directive itself, since nginx rejects a duplicate.
	absoluteRedirect := "absolute_redirect off;"
	if strings.Contains(extraConfig, "absolute_redirect") {
		absoluteRedirect = ""
	}

	return fmt.Sprintf(`
server {
  listen 80;
  server_name _;

  root /var/www/html;
  index index.php index.html;

  %s

  %s

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
`, absoluteRedirect, extraConfig, fastcgiHTTPS, fastcgiXFP)
}

// buildPHPIniContent renders the php.ini overrides from spec.php.config.
func buildPHPIniContent(site sitev1alpha1.Site) string {
	if site.Spec.Php == nil {
		return ""
	}
	return site.Spec.Php.Config
}

// configChecksum hashes the rendered config file contents so it can be set
// as a pod template annotation: changing only a mounted ConfigMap doesn't
// change the Deployment's pod template, so kubelet/nginx/php-fpm never see
// the update until something forces a new pod. Annotating the pod template
// with this hash makes an actual config change produce a new pod template
// too, triggering a real rollout.
func configChecksum(contents ...string) string {
	h := sha256.New()
	for _, c := range contents {
		h.Write([]byte(c))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func buildDeploymentSpec(site sitev1alpha1.Site, labels map[string]string, envs []corev1.EnvVar, configHash string) appsv1.DeploymentSpec {
	return appsv1.DeploymentSpec{
		Replicas: int32Ptr(1),
		Strategy: appsv1.DeploymentStrategy{
			Type: deploymentStrategyType(site),
		},
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
				Annotations: map[string]string{
					"site-operator.propastinv/config-hash": configHash,
				},
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
					{
						Name: "php-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: site.Name + "-php",
								},
							},
						},
					},
					{
						Name: "fb-config",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
				},
				InitContainers: []corev1.Container{
					buildWPInitContainer(site),
				},
				Containers: func() []corev1.Container {
					containers := []corev1.Container{
						buildPHPFPMContainer(site, envs),
						buildNginxContainer(site),
					}
					if site.Spec.FileBrowser != nil && site.Spec.FileBrowser.Enabled {
						containers = append(containers, buildFileBrowserContainer(site))
					}
					return containers
				}(),
			},
		},
	}
}

// deploymentStrategyType defaults to RollingUpdate (matching the CRD default
// and prior behavior) when unset, e.g. for Sites built directly in Go rather
// than read back through the API server's defaulting.
func deploymentStrategyType(site sitev1alpha1.Site) appsv1.DeploymentStrategyType {
	if site.Spec.UpdateStrategy == string(appsv1.RecreateDeploymentStrategyType) {
		return appsv1.RecreateDeploymentStrategyType
	}
	return appsv1.RollingUpdateDeploymentStrategyType
}

func buildWPInitContainer(site sitev1alpha1.Site) corev1.Container {
	return corev1.Container{
		Name:    "wp-init",
		Image:   phpImage(site),
		Command: []string{"sh", "-c"},
		Args: []string{`
set -e

if [ ! -f /var/www/html/wp ]; then
  echo "Downloading wp-cli..."
  # Use php to download if curl is not available
  php -r "copy('https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar', '/var/www/html/wp');"
  chmod +x /var/www/html/wp
fi

# Only seeds an empty volume; a site that has already started is never touched.
if [ ! -f /var/www/html/index.php ]; then
  echo "Initializing WordPress files (version: $WP_VERSION)..."
  if [ "$WP_VERSION" = "bundled" ]; then
    cp -r /usr/src/wordpress/* /var/www/html/
  else
    # memory_limit: wp-cli's archive extraction needs more than the 128M CLI default.
    # No fallback on purpose: if the download fails the pod fails to start (the
    # kubelet retries the init container) instead of silently running whatever
    # older WordPress the node's cached image happens to bundle.
    php -d memory_limit=512M /var/www/html/wp core download --path=/var/www/html --version="$WP_VERSION" --allow-root
  fi
fi

# Numeric on purpose: php-fpm and filebrowser run as 33:33 (see their
# securityContext), but "www-data" is uid 33 only on Debian-based images (it is
# 82 on Alpine ones), so chowning by name breaks spec.php.image with *-alpine.
chown -R 33:33 /var/www/html
`},
		Env: []corev1.EnvVar{
			{Name: "WP_VERSION", Value: wpVersion(site)},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: siteDataVolume, MountPath: siteRoot},
		},
	}
}

// wpVersion is the WordPress core version a new site is seeded with; see
// WordpressSpec.Version.
func wpVersion(site sitev1alpha1.Site) string {
	if site.Spec.Wordpress != nil && site.Spec.Wordpress.Version != "" {
		return site.Spec.Wordpress.Version
	}
	return "latest"
}

func buildPHPFPMContainer(site sitev1alpha1.Site, envs []corev1.EnvVar) corev1.Container {
	return corev1.Container{
		Name:    "php-fpm",
		Image:   phpImage(site),
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

// The operator's own wp-cron CronJob calls wp-cron.php directly on a
// schedule (see cmd/wpcron.go), so WordPress's own page-load-triggered
// pseudo-cron (unreliable on low-traffic sites, wasted work on high-traffic
// ones) is disabled here. This does not disable cron events themselves, only
// the implicit trigger on every page load.
define('DISABLE_WP_CRON', true);

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

if [ "$WP_INSTALL" = "true" ]; then
  echo "Checking if WordPress is installed..."
  
  # Wait for DB
  echo "Waiting for database connection..."
  until php -r "
    \$host = getenv('DB_HOST');
    \$user = getenv('DB_USER');
    \$pass = getenv('DB_PASSWORD');
    \$name = getenv('DB_NAME');
    \$conn = @new mysqli(\$host, \$user, \$pass, \$name);
    if (\$conn->connect_error) {
        fwrite(STDERR, 'Connection error: ' . \$conn->connect_error . PHP_EOL);
        exit(1);
    }
    exit(0);
  "; do
    sleep 2
  done
  echo "Database connection ready."
  
  if ! /var/www/html/wp core is-installed; then
    echo "Installing WordPress..."
    /var/www/html/wp core install \
      --url="$WP_HOME" \
      --title="$WP_TITLE" \
      --admin_user="$WP_ADMIN_USER" \
      --admin_password="$WP_ADMIN_PASSWORD" \
      --admin_email="$WP_ADMIN_EMAIL" \
      --skip-email
    echo "WordPress installed successfully."
  else
    echo "WordPress is already installed."
  fi
fi

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
			{Name: siteDataVolume, MountPath: siteRoot},
			{
				Name:      "php-config",
				MountPath: "/usr/local/etc/php/conf.d/" + phpIniFileName,
				SubPath:   phpIniFileName,
			},
		},
	}
}

func buildNginxContainer(site sitev1alpha1.Site) corev1.Container {
	return corev1.Container{
		Name:  "nginx",
		Image: nginxImage(site),
		Ports: []corev1.ContainerPort{
			{ContainerPort: 80},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: siteDataVolume, MountPath: siteRoot},
			{Name: "nginx-config", MountPath: "/etc/nginx/conf.d/default.conf", SubPath: "default.conf"},
		},
	}
}

func buildFileBrowserContainer(site sitev1alpha1.Site) corev1.Container {
	image := "filebrowser/filebrowser:latest"
	if site.Spec.FileBrowser != nil && site.Spec.FileBrowser.Image != "" {
		image = site.Spec.FileBrowser.Image
	}

	return corev1.Container{
		Name:    "filebrowser",
		Image:   image,
		Command: []string{"sh", "-c"},
		Args: []string{`
set -e
if [ ! -f "$FB_DATABASE" ]; then
  filebrowser config init --database="$FB_DATABASE" --address="$FB_ADDRESS" --port="$FB_PORT" --root="$FB_ROOT" --baseURL="$FB_BASE_URL" --fileMode=0o644 --dirMode=0o755
  filebrowser users add "$FB_USERNAME" "$FB_PASSWORD" --database="$FB_DATABASE" --perm.admin
fi
exec filebrowser
`},
		Env: []corev1.EnvVar{
			{Name: "FB_PORT", Value: "8080"},
			{Name: "FB_ROOT", Value: siteRoot},
			{Name: "FB_DATABASE", Value: "/tmp/filebrowser.db"},
			{Name: "FB_BASE_URL", Value: "/filebrowser"},
			{Name: "FB_ADDRESS", Value: "0.0.0.0"},
			{Name: "FB_USERNAME", Value: "admin"},
			secretEnv(site, "FB_PASSWORD"),
		},
		Ports: []corev1.ContainerPort{
			{ContainerPort: 8080},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:  int64Ptr(33),
			RunAsGroup: int64Ptr(33),
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: siteDataVolume, MountPath: siteRoot},
			{Name: "fb-config", MountPath: "/config"},
		},
	}
}

func buildSiteDataVolume(site sitev1alpha1.Site) corev1.Volume {

	if site.Spec.Persistence == nil || !site.Spec.Persistence.Enabled {
		return corev1.Volume{
			Name: siteDataVolume,
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
		Name: siteDataVolume,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: claimName,
			},
		},
	}
}
