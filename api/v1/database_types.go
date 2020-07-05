/*


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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=Postgres
type DatabaseType string

var (
	DatabaseTypePostgres DatabaseType = "Postgres"
)

// DatabaseSpec defines the desired state of Database
type DatabaseSpec struct {
	// Type is the type of the database, for example Postgres
	Type DatabaseType `json:"type,omitempty"`
	// Extensions are the list of extensions required
	// +optional
	Extensions []string `json:"extensions,omitempty"`
}

// DatabaseStatus defines the observed state of Database
type DatabaseStatus struct {
	Provisioned *bool `json:"provisioned"`
}

// +kubebuilder:object:root=true

// Database is the Schema for the databases API
type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseSpec   `json:"spec,omitempty"`
	Status DatabaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DatabaseList contains a list of Database
type DatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Database `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Database{}, &DatabaseList{})
}
