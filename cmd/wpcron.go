/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sitev1alpha1 "github.com/propastinv/site-operator/api/v1alpha1"
)

// wpCronHTTPTimeout bounds each individual site's wp-cron.php request, so one
// slow or hanging site can't stall the whole run.
const wpCronHTTPTimeout = 20 * time.Second

// runWPCronOnce triggers wp-cron.php on every Site's in-cluster Service and
// exits. It reads Sites directly from the API server (no cache/manager
// needed for a one-shot run) using the same ServiceAccount and RBAC as the
// controller. wp-config.php sets DISABLE_WP_CRON, so this is the only thing
// that runs WordPress's scheduled events; a failure on one site doesn't stop
// the others, but the run exits non-zero if any site failed so the CronJob's
// failure history stays meaningful.
func runWPCronOnce(ctx context.Context) error {
	c, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("building client: %w", err)
	}

	var sites sitev1alpha1.SiteList
	if err := c.List(ctx, &sites); err != nil {
		return fmt.Errorf("listing sites: %w", err)
	}

	httpClient := &http.Client{Timeout: wpCronHTTPTimeout}

	var failed []string
	for _, site := range sites.Items {
		url := fmt.Sprintf("http://%s.%s.svc.cluster.local/wp-cron.php?doing_wp_cron", site.Name, site.Namespace)

		reqCtx, cancel := context.WithTimeout(ctx, wpCronHTTPTimeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
		if err != nil {
			cancel()
			setupLog.Error(err, "building wp-cron request", "site", site.Name, "namespace", site.Namespace)
			failed = append(failed, site.Namespace+"/"+site.Name)
			continue
		}

		resp, err := httpClient.Do(req)
		cancel()
		if err != nil {
			setupLog.Error(err, "wp-cron request failed", "site", site.Name, "namespace", site.Namespace)
			failed = append(failed, site.Namespace+"/"+site.Name)
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode >= 400 {
			setupLog.Info("wp-cron returned an error status",
				"site", site.Name, "namespace", site.Namespace, "status", resp.StatusCode)
			failed = append(failed, site.Namespace+"/"+site.Name)
			continue
		}

		setupLog.Info("wp-cron triggered", "site", site.Name, "namespace", site.Namespace, "status", resp.StatusCode)
	}

	setupLog.Info("wp-cron run complete", "sites", len(sites.Items), "failed", len(failed))
	if len(failed) > 0 {
		return errors.New("wp-cron failed for: " + fmt.Sprint(failed))
	}
	return nil
}
