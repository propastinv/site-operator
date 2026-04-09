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
	// foo is an example field of Site. Edit site_types.go to remove/update
	// +optional
	Foo *string `json:"foo,omitempty"`
	// +kubebuilder:default=localhost
	Domain string `json:"domain,omitempty"`

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

// +kubebuilder:validation:XValidation:rule="((has(self.userSecret) && !has(self.user)) || (!has(self.userSecret) && has(self.user))) && ((has(self.passwordSecret) && !has(self.password)) || (!has(self.passwordSecret) && has(self.password)))",message="provide exactly one of userSecret or user, and exactly one of passwordSecret or password"
type DatabaseSpec struct {
	// +required
	Host string `json:"host"`
	// +required
	Name string `json:"name"`

	// Credentials can be provided either via secrets (userSecret/passwordSecret)
	// or directly as plain strings (user/password). Exactly one method must be used.
	// +optional
	UserSecret *SecretKeyRef `json:"userSecret,omitempty"`
	// +optional
	PasswordSecret *SecretKeyRef `json:"passwordSecret,omitempty"`
	// +optional
	User *string `json:"user,omitempty"`
	// +optional
	Password *string `json:"password,omitempty"`
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

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec SiteSpec `json:"spec"`

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

