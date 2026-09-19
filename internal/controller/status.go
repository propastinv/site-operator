package controller

import (
	appsv1 "k8s.io/api/apps/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	conditionAvailable   = "Available"
	conditionProgressing = "Progressing"
	conditionDegraded    = "Degraded"
)

// setSiteConditions derives Available/Progressing/Degraded conditions for a Site
// from the outcome of the last reconcile and its owned Deployment's status, and
// applies them via apimeta.SetStatusCondition so LastTransitionTime only moves
// when a condition's Status actually changes.
func setSiteConditions(conditions *[]metav1.Condition, generation int64, deploy *appsv1.Deployment, deployErr error, reconcileErr error) {
	switch {
	case reconcileErr != nil:
		apply(conditions, generation, "ReconcileError", reconcileErr.Error(),
			metav1.ConditionFalse, metav1.ConditionFalse, metav1.ConditionTrue)

	case deployErr != nil:
		apply(conditions, generation, "DeploymentNotFound", "site deployment does not exist yet",
			metav1.ConditionFalse, metav1.ConditionTrue, metav1.ConditionFalse)

	case deploymentReady(deploy):
		apply(conditions, generation, "DeploymentReady", "deployment has the desired number of ready replicas",
			metav1.ConditionTrue, metav1.ConditionFalse, metav1.ConditionFalse)

	default:
		apply(conditions, generation, "DeploymentRollingOut", "waiting for deployment to become ready",
			metav1.ConditionFalse, metav1.ConditionTrue, metav1.ConditionFalse)
	}
}

func deploymentReady(deploy *appsv1.Deployment) bool {
	desired := int32(1)
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}

	return deploy.Status.ObservedGeneration >= deploy.Generation &&
		deploy.Status.ReadyReplicas >= desired &&
		deploy.Status.UpdatedReplicas >= desired
}

func apply(conditions *[]metav1.Condition, generation int64, reason, message string, available, progressing, degraded metav1.ConditionStatus) {
	for _, c := range []struct {
		condType string
		status   metav1.ConditionStatus
	}{
		{conditionAvailable, available},
		{conditionProgressing, progressing},
		{conditionDegraded, degraded},
	} {
		apimeta.SetStatusCondition(conditions, metav1.Condition{
			Type:               c.condType,
			Status:             c.status,
			Reason:             reason,
			Message:            message,
			ObservedGeneration: generation,
		})
	}
}
