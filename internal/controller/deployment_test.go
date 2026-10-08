package controller

import (
	"strings"
	"testing"

	sitev1alpha1 "github.com/propastinv/site-operator/api/v1alpha1"
)

func TestWPVersion(t *testing.T) {
	cases := []struct {
		name string
		spec sitev1alpha1.SiteSpec
		want string
	}{
		{"unset defaults to latest", sitev1alpha1.SiteSpec{}, "latest"},
		{"wordpress without version defaults to latest", sitev1alpha1.SiteSpec{Wordpress: &sitev1alpha1.WordpressSpec{}}, "latest"},
		{"pinned", sitev1alpha1.SiteSpec{Wordpress: &sitev1alpha1.WordpressSpec{Version: "6.9.4"}}, "6.9.4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			site := sitev1alpha1.Site{Spec: tc.spec}
			if got := wpVersion(site); got != tc.want {
				t.Errorf("wpVersion = %q, want %q", got, tc.want)
			}
			var env string
			for _, e := range buildWPInitContainer(site).Env {
				if e.Name == "WP_VERSION" {
					env = e.Value
				}
			}
			if env != tc.want {
				t.Errorf("wp-init WP_VERSION env = %q, want %q", env, tc.want)
			}
		})
	}
}

func TestNginxConfigAbsoluteRedirect(t *testing.T) {
	count := func(site sitev1alpha1.Site) int {
		return strings.Count(buildNginxConfigContent(site), "absolute_redirect")
	}

	if got := count(sitev1alpha1.Site{}); got != 1 {
		t.Errorf("default config has %d absolute_redirect directives, want exactly 1", got)
	}
	if !strings.Contains(buildNginxConfigContent(sitev1alpha1.Site{}), "absolute_redirect off;") {
		t.Error("default config must turn absolute_redirect off so nginx redirects keep the client's scheme")
	}

	// A site that already works around this itself must not end up with a
	// duplicate directive (nginx refuses to start with one).
	own := sitev1alpha1.Site{Spec: sitev1alpha1.SiteSpec{
		Nginx: &sitev1alpha1.NginxSpec{Config: "absolute_redirect off;"},
	}}
	if got := count(own); got != 1 {
		t.Errorf("config with the site's own absolute_redirect has %d directives, want exactly 1", got)
	}
}

func TestNginxConfigForwardedProto(t *testing.T) {
	tls := true
	plain := false
	cases := []struct {
		name    string
		ingress *sitev1alpha1.IngressSpec
		want    bool
	}{
		{"tls site tells PHP it is served over https", &sitev1alpha1.IngressSpec{TLS: &tls}, true},
		{"explicit tls=false sets nothing", &sitev1alpha1.IngressSpec{TLS: &plain}, false},
		{"no ingress sets nothing", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conf := buildNginxConfigContent(sitev1alpha1.Site{Spec: sitev1alpha1.SiteSpec{Ingress: tc.ingress}})

			if got := strings.Contains(conf, "fastcgi_param HTTP_X_FORWARDED_PROTO https;"); got != tc.want {
				t.Errorf("X-Forwarded-Proto https set = %v, want %v", got, tc.want)
			}
			// nginx's $scheme is always http behind a TLS-terminating ingress, so it
			// must never be what PHP is told the client scheme is.
			if strings.Contains(conf, "HTTP_X_FORWARDED_PROTO $scheme") {
				t.Error("X-Forwarded-Proto must not be derived from nginx's $scheme")
			}
			if tc.want != strings.Contains(conf, "fastcgi_param HTTPS on;") {
				t.Error("HTTPS on and X-Forwarded-Proto https must be set together")
			}
		})
	}
}
