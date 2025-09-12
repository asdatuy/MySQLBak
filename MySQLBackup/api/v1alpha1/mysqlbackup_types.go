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
	Endpoint string `json:"endpoint"`
	// +required
	Bucket string `json:"bucket"`
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
	DBAuth string `json:"dbAuth"`
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
	// +optional
	LastBakStatus string `json:"lastBakStatus,omitempty"`
	// +optional
	LastReason string `json:"lastReason,omitempty"`
	// +optional
	LatestCreateTime *metav1.Time `json:"lastCreateTime,omitempty"`
	// +optional
	LatestSucceedTime *metav1.Time `json:"lastSucceedTime,omitempty"`
	// +optional
	LatestFailedTime *metav1.Time `json:"lastFailedTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="LastStatus",type=string,JSONPath=`.status.lastBakStatus`
// +kubebuilder:printcolumn:name="LastReason",type=string,JSONPath=`.status.lastReason`
// +kubebuilder:printcolumn:name="LastSucceed",type=string,JSONPath=`.status.lastSucceedTime`
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
