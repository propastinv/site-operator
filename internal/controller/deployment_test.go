package controller

import (
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
