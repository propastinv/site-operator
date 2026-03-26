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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SiteSpec defines the desired state of Site
type SiteSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// foo is an example field of Site. Edit site_types.go to remove/update
	// +optional
	Foo *string `json:"foo,omitempty"`
	// +kubebuilder:default=localhost
	Domain    string         `json:"domain,omitempty"`
	Ingress   *IngressSpec   `json:"ingress,omitempty"`
	Wordpress *WordpressSpec `json:"wordpress,omitempty"`
	// +required
	Database DatabaseSpec `json:"database"`
	// +optional
	Persistence *PersistenceSpec `json:"persistence,omitempty"`
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

type WordpressSpec struct {
	Debug *DebugSpec `json:"debug,omitempty"`
}

type DebugSpec struct {
	Enabled bool `json:"enabled,omitempty"`
	Log     bool `json:"log,omitempty"`
	Display bool `json:"display,omitempty"`
}

type DatabaseSpec struct {
	// +required
	Host string `json:"host"`
	// +required
	Name string `json:"name"`
	// +required
	UserSecret *SecretKeyRef `json:"userSecret"`
	// +required
	PasswordSecret *SecretKeyRef `json:"passwordSecret"`
}

type SecretKeyRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type IngressSpec struct {
	Enabled          bool              `json:"enabled,omitempty"`
	Path             string            `json:"path,omitempty"`
	TLS              *bool             `json:"tls,omitempty"`
	Annotations      map[string]string `json:"annotations,omitempty"`
	IngressClassName string            `json:"ingressClassName,omitempty"`
}

type PersistenceSpec struct {
	Enabled bool `json:"enabled,omitempty"`

	// +optional
	ExistingClaim string `json:"existingClaim,omitempty"`

	// +kubebuilder:default="1Gi"
	Size string `json:"size,omitempty"`

	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

// SiteStatus defines the observed state of Site.
type SiteStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the Site resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Site is the Schema for the sites API
type Site struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Site
	// +required
	Spec SiteSpec `json:"spec"`

	// status defines the observed state of Site
	// +optional
	Status SiteStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SiteList contains a list of Site
type SiteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Site `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Site{}, &SiteList{})
}
