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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// StorageGroupSpec defines the desired state of StorageGroup
type StorageGroupSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Name of the StorageClass associated with this Storage Group
	StorageClassName string `json:"storageClassName,omitempty"`

	// Configs for setting the allowed sizes of PVCs
	SizeConfig StorageGroupSizeConfig `json:"sizeConfig,omitempty"`
}

type StorageGroupSizeConfig struct {
	// Allow users to set custom sizes for PVCs created using this StorageGroup
	// +kubebuilder:default:=false
	Custom bool `json:"custom,omitempty"`

	// List of sizes of PVCs the user can select from for this StorageGroup
	Options []StorageGroupSizeOption `json:"options,omitempty"`
}

// +kubebuilder:validation:Pattern=`^[0-9]{1,4}[MGT][IBib]$`
type StorageGroupSizeOption string

// StorageGroupStatus defines the observed state of StorageGroup
type StorageGroupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// StorageGroup is the Schema for the storagegroups API
type StorageGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StorageGroupSpec   `json:"spec,omitempty"`
	Status StorageGroupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// StorageGroupList contains a list of StorageGroup
type StorageGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StorageGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StorageGroup{}, &StorageGroupList{})
}
