package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	StorageModeS3 StorageMode = "s3"
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
}

type AvailableVersion struct {
	// +required
	Path string `json:"path,omitzero"`
	// +required
	Server string `json:"server,omitzero"`
	// +required
	// +default=""
	Available string `json:"available,omitzero"`
	// +optional
	BackTimeStep string `json:"backTimeStep,omitzero"`
}

type MySQLBackupStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitzero" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	// +optional
	LastBakStatus string `json:"lastBakStatus,omitzero"`
	// +optional
	AvailableVersion []AvailableVersion `json:"availableVersion,omitzero"`
	// +optional
	LastReason string `json:"lastReason,omitzero"`
	// +required
	LatestActiveTime metav1.Time `json:"lastActiveTime,omitzero"`
	// +required
	LatestSucceedTime metav1.Time `json:"lastSucceedTime,omitzero"`
	// +required
	LatestFailedTime metav1.Time `json:"lastFailedTime,omitzero"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="LastStatus",type=string,JSONPath=`.status.lastBakStatus`
// +kubebuilder:printcolumn:name="LastReason",type=string,JSONPath=`.status.lastReason`
// +kubebuilder:printcolumn:name="LastSucceed",type=string,JSONPath=`.status.lastSucceedTime`
type MySQLBackup struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`
	// +required
	Spec MySQLBackupSpec `json:"spec"`
	// +optional
	Status MySQLBackupStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true
type MySQLBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []MySQLBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MySQLBackup{}, &MySQLBackupList{})
}
