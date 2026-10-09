package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sitev1alpha1 "github.com/propastinv/site-operator/api/v1alpha1"
)

func testSite(name string) sitev1alpha1.Site {
	return sitev1alpha1.Site{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"}}
}

func TestTriggerSitesToleratesUnreachableSites(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer ok.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer broken.Close()
	// A server that is already closed: the same "connection refused" a Service
	// without ready endpoints gives while its pod is restarting.
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	closedURL := closed.URL
	closed.Close()

	urls := map[string]string{"up": ok.URL, "restarting": closedURL, "php-fatal": broken.URL}
	sites := []sitev1alpha1.Site{testSite("up"), testSite("restarting"), testSite("php-fatal"), testSite("up")}

	httpClient := &http.Client{Timeout: 2 * time.Second}
	triggered, unreachable := triggerSites(context.Background(), httpClient, sites,
		func(s sitev1alpha1.Site) string { return urls[s.Name] })

	if triggered != 2 {
		t.Errorf("triggered = %d, want 2 (a failing site must not stop the others)", triggered)
	}
	if len(unreachable) != 2 {
		t.Errorf("unreachable = %v, want the restarting and php-fatal sites", unreachable)
	}
	if err := cronRunError(len(sites), triggered, unreachable); err != nil {
		t.Errorf("one unreachable site must not fail the run, got: %v", err)
	}
}

func TestCronRunError(t *testing.T) {
	cases := []struct {
		name             string
		total, triggered int
		wantErr          bool
	}{
		{"no sites at all is fine", 0, 0, false},
		{"all sites triggered", 3, 3, false},
		{"some sites unreachable is fine", 3, 1, false},
		{"every site unreachable looks systemic", 3, 0, true},
		{"single site unreachable looks systemic", 1, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := cronRunError(tc.total, tc.triggered, nil)
			if (err != nil) != tc.wantErr {
				t.Errorf("cronRunError(%d, %d) error = %v, wantErr %v", tc.total, tc.triggered, err, tc.wantErr)
			}
		})
	}
}
