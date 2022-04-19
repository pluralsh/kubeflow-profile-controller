/*
Copyright 2021.

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
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type EnvironmentType string

const (
	ProjectEnvironment  EnvironmentType = "Project"
	UserEnvironment     EnvironmentType = "User"
	PersonalEnvironment EnvironmentType = "Personal"
)

// EnvironmentSpec defines the desired state of Environment
type EnvironmentSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Enum=Project;User;Personal
	Type EnvironmentType `json:"type,omitempty"`
	// +kubebuilder:validation:Optional
	ParentProject string             `json:"parentProject,omitempty"`
	Owners        EnvironementOwners `json:"owners,omitempty"`
	Permissions   rbacv1.RoleRef     `json:"permissions,omitempty"`
}

type EnvironementOwners struct {
	Users []string `json:"users,omitempty"`
}

type EnvironmentCondition struct {
	Type    EnvironmentConditionType `json:"type,omitempty"`
	Status  string                   `json:"status,omitempty" description:"status of the condition, one of True, False, Unknown"`
	Message string                   `json:"message,omitempty"`
}

type EnvironmentConditionType string

const (
	EnvironmentSucceed EnvironmentConditionType = "Successful"
	EnvironmentFailed  EnvironmentConditionType = "Failed"
	EnvironmentUnknown EnvironmentConditionType = "Unknown"
)

// EnvironmentStatus defines the observed state of Environment
type EnvironmentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Conditions []EnvironmentCondition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=environments,scope=Cluster

// Environment is the Schema for the environments API
type Environment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnvironmentSpec   `json:"spec,omitempty"`
	Status EnvironmentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EnvironmentList contains a list of Environment
type EnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Environment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Environment{}, &EnvironmentList{})
}
