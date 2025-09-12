package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DbSpec struct {
	// +required
	DbHost string `json:"dbHost"`
	// +optional
	// +default=3306
	DbPort string `json:"dbPort"`
	// +required
	DbName string `json:"dbName"`
	// +required
	DbAuth string `json:"dbAuth"`
}

type S3Spec struct {
	// +required
	Endpoint string `json:"endpoint"`
	// +required
	Bucket string `json:"bucket"`
	// +required
	S3Auth string `json:"s3Auth"`
	// +optional
	RestoreVid *string `json:"restoreVid,omitempty"`
}

type MySQLRestoreSpec struct {
	// +required
	DbSpec DbSpec `json:"DbSpec"`
	// +required
	S3Spec S3Spec `json:"S3Spec"`
}

type LastRestoreSpce struct {
	// +requird
	DbHost string `json:"dbHost"`
	// +requird
	DbName string `json:"dbName"`
	// +required
	RestoreVerison string `json:"restoreVersion"`
}

type MySQLRestoreStatus struct {
	// +required
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	// +optional
	LastRestoreSpec LastRestoreSpce `json:"lastRestoreSpec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type MySQLRestore struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`
	// +required
	Spec MySQLRestoreSpec `json:"spec"`
	// +optional
	Status MySQLRestoreStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true
type MySQLRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MySQLRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MySQLRestore{}, &MySQLRestoreList{})
}
