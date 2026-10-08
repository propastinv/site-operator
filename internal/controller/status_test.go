package controller

import (
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetSiteConditions(t *testing.T) {
	one := int32(1)
	deploy := func(status appsv1.DeploymentStatus) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Generation: 3},
			Spec:       appsv1.DeploymentSpec{Replicas: &one},
			Status:     status,
		}
	}
	progressDeadline := appsv1.DeploymentCondition{
		Type:    appsv1.DeploymentProgressing,
		Status:  corev1.ConditionFalse,
		Reason:  "ProgressDeadlineExceeded",
		Message: `ReplicaSet "x" has timed out progressing.`,
	}

	cases := []struct {
		name                                  string
		deploy                                *appsv1.Deployment
		deployErr, reconcileErr               error
		reason                                string
		available, progressing, degradedState metav1.ConditionStatus
	}{
		{
			name: "fully rolled out",
			deploy: deploy(appsv1.DeploymentStatus{
				ObservedGeneration: 3, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1,
			}),
			reason:    reasonReady,
			available: metav1.ConditionTrue, progressing: metav1.ConditionFalse, degradedState: metav1.ConditionFalse,
		},
		{
			// The case that used to be reported as healthy: the old pod is still
			// ready while the new revision's pod can't start (bad image).
			name: "old pod serving, new pod not available yet",
			deploy: deploy(appsv1.DeploymentStatus{
				ObservedGeneration: 3, Replicas: 2, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1,
			}),
			reason:    reasonRollingOut,
			available: metav1.ConditionTrue, progressing: metav1.ConditionTrue, degradedState: metav1.ConditionFalse,
		},
		{
			name: "rollout stalled past its progress deadline",
			deploy: deploy(appsv1.DeploymentStatus{
				ObservedGeneration: 3, Replicas: 2, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1,
				Conditions: []appsv1.DeploymentCondition{progressDeadline},
			}),
			reason:    reasonRolloutStalled,
			available: metav1.ConditionTrue, progressing: metav1.ConditionFalse, degradedState: metav1.ConditionTrue,
		},
		{
			name: "first start, nothing available yet",
			deploy: deploy(appsv1.DeploymentStatus{
				ObservedGeneration: 3, Replicas: 1, UpdatedReplicas: 1,
			}),
			reason:    reasonRollingOut,
			available: metav1.ConditionFalse, progressing: metav1.ConditionTrue, degradedState: metav1.ConditionFalse,
		},
		{
			name: "controller has not observed the latest spec yet",
			deploy: deploy(appsv1.DeploymentStatus{
				ObservedGeneration: 2, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1,
			}),
			reason:    reasonRollingOut,
			available: metav1.ConditionTrue, progressing: metav1.ConditionTrue, degradedState: metav1.ConditionFalse,
		},
		{
			name:      "deployment missing",
			deploy:    &appsv1.Deployment{},
			deployErr: errors.New("not found"),
			reason:    reasonNotFound,
			available: metav1.ConditionFalse, progressing: metav1.ConditionTrue, degradedState: metav1.ConditionFalse,
		},
		{
			name:         "reconcile error wins",
			deploy:       &appsv1.Deployment{},
			reconcileErr: errors.New("boom"),
			reason:       reasonReconcileError,
			available:    metav1.ConditionFalse, progressing: metav1.ConditionFalse, degradedState: metav1.ConditionTrue,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var conds []metav1.Condition
			setSiteConditions(&conds, 7, tc.deploy, tc.deployErr, tc.reconcileErr)

			for typ, want := range map[string]metav1.ConditionStatus{
				conditionAvailable:   tc.available,
				conditionProgressing: tc.progressing,
				conditionDegraded:    tc.degradedState,
			} {
				got := apimeta.FindStatusCondition(conds, typ)
				if got == nil {
					t.Fatalf("condition %s not set", typ)
				}
				if got.Status != want {
					t.Errorf("%s = %s, want %s", typ, got.Status, want)
				}
				if got.Reason != tc.reason {
					t.Errorf("%s reason = %s, want %s", typ, got.Reason, tc.reason)
				}
			}
		})
	}
}
