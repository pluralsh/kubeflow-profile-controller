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

// ConfigSpec defines the desired state of Config
type ConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Configs related to namespaces
	Namespace NamespaceSpec `json:"namespace,omitempty"`

	// Network related configs
	Network NetworkSpec `json:"network,omitempty"`

	// Security related configs
	Security SecuritySpec `json:"security,omitempty"`

	// Identity related configs
	Identity IdentitySpec `json:"identity,omitempty"`

	// Email addresses of Kubeflow cluster level admins
	Admins []string `json:"admins,omitempty"`
}

type NamespaceSpec struct {
	// Prefix to add to all created namespaces
	Prefix string `json:"prefix,omitempty"`

	// Labels to add to all created namespaces
	DefaultLabels map[string]string `json:"defaultLabels,omitempty"`
}

type NetworkSpec struct {
	// Hostname of the Kubeflow deployment
	Hostname string `json:"hostname,omitempty"`

	// Domain for the local cluster
	//+kubebuilder:default:="cluster.local"
	ClusterDomain string `json:"clusterDomain,omitempty"`

	// Istio related configs
	Istio IstioSpec `json:"istio,omitempty"`
}

type IstioSpec struct {
	// Istio Gateway resource used for Kubeflow
	ClusterGateway IstioGatewaySpec `json:"clusterGateway,omitempty"`
}

type IstioGatewaySpec struct {
	// Name of the Istio gateway to use for Kubeflow
	Name string `json:"name,omitempty"`

	// Namespace of the Istio gateway to use for Kubeflow
	Namespace string `json:"namespace,omitempty"`
}

type SecuritySpec struct {
	// Settings related to OIDC configuration
	OIDC OIDCSpec `json:"oidc,omitempty"`
}

type OIDCSpec struct {
	// The OIDC issuer to setup with Istio
	Issuer string `json:"issuer,omitempty"`

	// The JWKS URI for the OIDC issuer you would like to use with Istio
	JwksURI string `json:"jwksURI,omitempty"`
}

type IdentitySpec struct {
	//+kubebuilder:validation:Optional
	// Prefix found in the JWT email claim
	UserIDPrefix string `json:"userIdPrefix,omitempty"`
}

// ConfigStatus defines the observed state of Config
type ConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=configs,scope=Cluster

// Config is the Schema for the configs API
type Config struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigSpec   `json:"spec,omitempty"`
	Status ConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ConfigList contains a list of Config
type ConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Config `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Config{}, &ConfigList{})
}
