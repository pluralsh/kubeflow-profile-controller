// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// ----------------------------------------------------------------------------
//
//     ***     AUTO GENERATED CODE    ***    AUTO GENERATED CODE     ***
//
// ----------------------------------------------------------------------------
//
//     This file is automatically generated by Config Connector and manual
//     changes will be clobbered when the file is regenerated.
//
// ----------------------------------------------------------------------------

// *** DISCLAIMER ***
// Config Connector's go-client for CRDs is currently in ALPHA, which means
// that future versions of the go-client may include breaking changes.
// Please try it out and give us feedback!

package v1beta1

import (
	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/k8s/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type IAMServiceAccountKeySpec struct {
	/* Immutable. The algorithm used to generate the key, used only on create. KEY_ALG_RSA_2048 is the default algorithm. Valid values are: "KEY_ALG_RSA_1024", "KEY_ALG_RSA_2048". */
	// +optional
	KeyAlgorithm *string `json:"keyAlgorithm,omitempty"`

	/* Immutable. */
	// +optional
	PrivateKeyType *string `json:"privateKeyType,omitempty"`

	/* Immutable. A field that allows clients to upload their own public key. If set, use this public key data to create a service account key for given service account. Please note, the expected format for this field is a base64 encoded X509_PEM. */
	// +optional
	PublicKeyData *string `json:"publicKeyData,omitempty"`

	/* Immutable. */
	// +optional
	PublicKeyType *string `json:"publicKeyType,omitempty"`

	/*  */
	ServiceAccountRef v1alpha1.ResourceRef `json:"serviceAccountRef"`
}

type IAMServiceAccountKeyStatus struct {
	/* Conditions represent the latest available observations of the
	   IAMServiceAccountKey's current state. */
	Conditions []v1alpha1.Condition `json:"conditions,omitempty"`
	/* Immutable. The name used for this key pair. */
	Name string `json:"name,omitempty"`
	/* ObservedGeneration is the generation of the resource that was most recently observed by the Config Connector controller. If this is equal to metadata.generation, then that means that the current reported status reflects the most recent desired state of the resource. */
	ObservedGeneration int `json:"observedGeneration,omitempty"`
	/* The private key in JSON format, base64 encoded. This is what you normally get as a file when creating service account keys through the CLI or web console. This is only populated when creating a new key. */
	PrivateKey string `json:"privateKey,omitempty"`
	/* Immutable. The public key, base64 encoded. */
	PublicKey string `json:"publicKey,omitempty"`
	/* The key can be used after this timestamp. A timestamp in RFC3339 UTC "Zulu" format, accurate to nanoseconds. Example: "2014-10-02T15:01:23.045123456Z". */
	ValidAfter string `json:"validAfter,omitempty"`
	/* The key can be used before this timestamp. A timestamp in RFC3339 UTC "Zulu" format, accurate to nanoseconds. Example: "2014-10-02T15:01:23.045123456Z". */
	ValidBefore string `json:"validBefore,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IAMServiceAccountKey is the Schema for the iam API
// +k8s:openapi-gen=true
type IAMServiceAccountKey struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IAMServiceAccountKeySpec   `json:"spec,omitempty"`
	Status IAMServiceAccountKeyStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IAMServiceAccountKeyList contains a list of IAMServiceAccountKey
type IAMServiceAccountKeyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IAMServiceAccountKey `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IAMServiceAccountKey{}, &IAMServiceAccountKeyList{})
}
