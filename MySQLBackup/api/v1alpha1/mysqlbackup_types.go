package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	StorageModeS3 StorageMode = "s3"
	StorageModePV StorageMode = "pv"
)

type StorageMode string

type S3BakMode struct {
	// +required
	Path string `json:"path"`
	// +required
	S3Auth string `json:"s3Auth"`
}

type PVBakMode struct {
	// +required
	Path string `json:"path"`
	// +required
	PvName string `json:"pvName"`
	// +optional
	SCName *string `json:"scName"`
}

type MySQLBackupSpec struct {
	// +required
	Host string `json:"host"`
	// +required
	DBAuth string `json:"secretName"`
	// +required
	// +default="3306"
	Port string `json:"port"`
	// +required
	// +default=false
	ManualTrigger bool `json:"manualTrigger"`
	// +required
	DBName string `json:"dbName"`
	// +required
	// +default="pv"
	BakMode StorageMode `json:"bakMode"`
	// +optional
	S3Bak *S3BakMode `json:"s3Bak"`
	// +optional
	PvBak *PVBakMode `json:"pvBak"`
}

type MySQLBackupStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[-1].status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[-1].reason`

type MySQLBackup struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// +required
	Spec MySQLBackupSpec `json:"spec"`

	// +optional
	Status MySQLBackupStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

type MySQLBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MySQLBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MySQLBackup{}, &MySQLBackupList{})
}
