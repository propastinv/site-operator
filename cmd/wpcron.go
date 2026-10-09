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
// that runs WordPress's scheduled events.
//
// A site that can't be reached (pod restarting, Recreate rollout, ...) is
// logged and skipped, not treated as a failure: WordPress runs overdue events
// on the next trigger, which is at most one schedule interval away. The run
// only fails when the mechanism itself looks broken, see cronRunError.
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
	triggered, unreachable := triggerSites(ctx, httpClient, sites.Items, siteCronURL)

	setupLog.Info("wp-cron run complete",
		"sites", len(sites.Items), "triggered", triggered, "unreachable", len(unreachable))
	return cronRunError(len(sites.Items), triggered, unreachable)
}

// siteCronURL is the in-cluster wp-cron.php endpoint of a Site's Service.
func siteCronURL(site sitev1alpha1.Site) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local/wp-cron.php?doing_wp_cron", site.Name, site.Namespace)
}

// triggerSites requests wp-cron.php for every site, one after another, and
// returns how many succeeded and the namespace/name of those that didn't.
// Problems are logged as plain messages (no stack traces): they're expected
// during rollouts and are retried on the next run.
func triggerSites(
	ctx context.Context,
	httpClient *http.Client,
	sites []sitev1alpha1.Site,
	urlFor func(sitev1alpha1.Site) string,
) (triggered int, unreachable []string) {
	for _, site := range sites {
		status, err := triggerWPCron(ctx, httpClient, urlFor(site))
		if err != nil {
			setupLog.Info("wp-cron not triggered, will retry on the next run",
				"site", site.Name, "namespace", site.Namespace, "reason", err.Error())
			unreachable = append(unreachable, site.Namespace+"/"+site.Name)
			continue
		}
		setupLog.Info("wp-cron triggered", "site", site.Name, "namespace", site.Namespace, "status", status)
		triggered++
	}
	return triggered, unreachable
}

// triggerWPCron issues one wp-cron.php request and reports an error if the
// site couldn't be reached or answered with an HTTP error.
func triggerWPCron(ctx context.Context, httpClient *http.Client, url string) (int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, wpCronHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return resp.StatusCode, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// cronRunError decides whether the run as a whole failed, so the CronJob (and
// anything alerting on failed Jobs) only reacts to something systemic, not to
// a single site being down. It fails only when there were sites to trigger and
// none could be: that points at cluster networking, a NetworkPolicy or a bad
// image rather than at any one site.
func cronRunError(total, triggered int, unreachable []string) error {
	if total > 0 && triggered == 0 {
		return fmt.Errorf("wp-cron could not reach any site (%d): %v", total, unreachable)
	}
	return nil
}
