package controller

import (
	"context"
	"fmt"

	mariadbv1alpha1 "github.com/mariadb-operator/mariadb-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	sitev1alpha1 "github.com/propastinv/site-operator/api/v1alpha1"
)

// provisionEnabled reports whether a Site opted into database provisioning
// via spec.provision.database.enabled. Every provisioning code path is gated
// behind this check, so mariadb-operator's CRDs are never required to be
// installed for Sites that don't use this feature.
func provisionEnabled(site sitev1alpha1.Site) bool {
	return site.Spec.Provision != nil &&
		site.Spec.Provision.Database != nil &&
		site.Spec.Provision.Database.Enabled
}

// provisionedUsername derives a MariaDB-safe username from the Site name.
// MySQL/MariaDB usernames are historically capped at 32 characters; "site_"
// (5 chars) plus up to 27 characters of the Site name stays under that limit.
func provisionedUsername(site sitev1alpha1.Site) string {
	name := site.Name
	if len(name) > 27 {
		name = name[:27]
	}
	return "site_" + name
}

// reconcileDatabaseProvision creates the mariadb-operator Database/User/Grant
// objects needed to provision credentials for an existing MariaDB cluster
// referenced by spec.provision.database.mariadbRef. It is a no-op unless the
// Site explicitly opts in, so this integration stays fully optional.
func reconcileDatabaseProvision(ctx context.Context, c client.Client, scheme *runtime.Scheme, owner metav1.Object, site sitev1alpha1.Site) error {
	if !provisionEnabled(site) {
		return nil
	}

	ref := site.Spec.Provision.Database.MariaDBRef
	mariaDBRef := mariadbv1alpha1.MariaDBRef{
		ObjectReference: mariadbv1alpha1.ObjectReference{
			Name:      ref.Name,
			Namespace: ref.Namespace,
		},
		WaitForIt: true,
	}
	username := provisionedUsername(site)

	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		db := &mariadbv1alpha1.Database{
			ObjectMeta: metav1.ObjectMeta{
				Name:      site.Name,
				Namespace: site.Namespace,
			},
		}
		_, err := controllerutil.CreateOrUpdate(ctx, c, db, func() error {
			db.Spec.MariaDBRef = mariaDBRef
			db.Spec.Name = site.Spec.Database.Name
			return controllerutil.SetControllerReference(owner, db, scheme)
		})
		return err
	}); err != nil {
		return fmt.Errorf("reconciling mariadb Database: %w", err)
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		user := &mariadbv1alpha1.User{
			ObjectMeta: metav1.ObjectMeta{
				Name:      site.Name + "-db-user",
				Namespace: site.Namespace,
			},
		}
		_, err := controllerutil.CreateOrUpdate(ctx, c, user, func() error {
			user.Spec.MariaDBRef = mariaDBRef
			user.Spec.Name = username
			user.Spec.PasswordSecretKeyRef = &mariadbv1alpha1.SecretKeySelector{
				LocalObjectReference: mariadbv1alpha1.LocalObjectReference{
					Name: site.Name + "-site-secret",
				},
				Key: "DB_PASSWORD",
			}
			return controllerutil.SetControllerReference(owner, user, scheme)
		})
		return err
	}); err != nil {
		return fmt.Errorf("reconciling mariadb User: %w", err)
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		grant := &mariadbv1alpha1.Grant{
			ObjectMeta: metav1.ObjectMeta{
				Name:      site.Name + "-db-grant",
				Namespace: site.Namespace,
			},
		}
		_, err := controllerutil.CreateOrUpdate(ctx, c, grant, func() error {
			grant.Spec.MariaDBRef = mariaDBRef
			grant.Spec.Privileges = []string{"ALL PRIVILEGES"}
			grant.Spec.Database = site.Spec.Database.Name
			grant.Spec.Table = "*"
			grant.Spec.Username = username
			return controllerutil.SetControllerReference(owner, grant, scheme)
		})
		return err
	}); err != nil {
		return fmt.Errorf("reconciling mariadb Grant: %w", err)
	}

	return nil
}
