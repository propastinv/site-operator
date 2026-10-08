package controller

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Reasons set on the Available/Progressing/Degraded conditions.
const (
	reasonReconcileError = "ReconcileError"
	reasonNotFound       = "DeploymentNotFound"
	reasonReady          = "DeploymentReady"
	reasonRollingOut     = "DeploymentRollingOut"
	reasonRolloutStalled = "RolloutStalled"
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
		apply(conditions, generation, reasonReconcileError, reconcileErr.Error(),
			metav1.ConditionFalse, metav1.ConditionFalse, metav1.ConditionTrue)

	case deployErr != nil:
		apply(conditions, generation, reasonNotFound, "site deployment does not exist yet",
			metav1.ConditionFalse, metav1.ConditionTrue, metav1.ConditionFalse)

	case deploymentRolledOut(deploy):
		apply(conditions, generation, reasonReady, "deployment has the desired number of ready replicas",
			metav1.ConditionTrue, metav1.ConditionFalse, metav1.ConditionFalse)

	default:
		// Not fully rolled out. Available stays truthful about whether any pod is
		// serving traffic (the old revision keeps serving during a RollingUpdate),
		// so a stuck rollout reads as "Available but Degraded", not as an outage.
		available := conditionStatus(deploy.Status.AvailableReplicas > 0)
		if stalled, message := deploymentStalled(deploy); stalled {
			apply(conditions, generation, reasonRolloutStalled, message,
				available, metav1.ConditionFalse, metav1.ConditionTrue)
		} else {
			apply(conditions, generation, reasonRollingOut, "waiting for deployment to become ready",
				available, metav1.ConditionTrue, metav1.ConditionFalse)
		}
	}
}

func conditionStatus(b bool) metav1.ConditionStatus {
	if b {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

// deploymentRolledOut mirrors `kubectl rollout status`: the new revision is
// fully rolled out only when every replica is updated, no old-revision pod is
// left, and all of them are available. Counting just ReadyReplicas and
// UpdatedReplicas is not enough: with a RollingUpdate, "old pod still ready +
// new pod updated but crash-looping" satisfies both and would look healthy.
func deploymentRolledOut(deploy *appsv1.Deployment) bool {
	desired := int32(1)
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}

	return deploy.Status.ObservedGeneration >= deploy.Generation &&
		deploy.Status.UpdatedReplicas >= desired &&
		deploy.Status.Replicas == deploy.Status.UpdatedReplicas &&
		deploy.Status.AvailableReplicas == deploy.Status.UpdatedReplicas
}

// deploymentStalled reports a rollout that exceeded the Deployment's progress
// deadline, e.g. a new pod stuck in ImagePullBackOff after a bad image.
func deploymentStalled(deploy *appsv1.Deployment) (bool, string) {
	for _, c := range deploy.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing && c.Status == corev1.ConditionFalse &&
			c.Reason == "ProgressDeadlineExceeded" {
			return true, c.Message
		}
	}
	return false, ""
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
