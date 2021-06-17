/*
Copyright The Kubernetes Authors.

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

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ClusterFinalizer allows ReconcileAzureIPPool to clean up Azure resources associated with AzureIPPool before
	// removing it from the apiserver.
	IPPoolFinalizer = "azureippool.infrastructure.cluster.x-k8s.io"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AzureIPPoolSpec defines the desired state of AzureIPPool
type AzureIPPoolSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of AzureIPPool. Edit AzureIPPool_types.go to remove/update
	Name string `json:"name"`

	IPPool []string `json:"ipPool,omitempty"`

	AllocatedIPs []string `json:"allocatedIps,omitempty"`

	VnetName string `json:"vnetName"`

	SubnetName string `json:"subnetName"`
}

// AzureIPPoolStatus defines the observed state of AzureIPPool
type AzureIPPoolStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true

// AzureIPPool is the Schema for the azureippools API
type AzureIPPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AzureIPPoolSpec   `json:"spec,omitempty"`
	Status AzureIPPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AzureIPPoolList contains a list of AzureIPPool
type AzureIPPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AzureIPPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AzureIPPool{}, &AzureIPPoolList{})
}
