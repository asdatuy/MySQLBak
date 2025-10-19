package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ResoterMode string

const (
	NewTargetMode    ResoterMode = "NewTargetMode"
	SourceTargetMode ResoterMode = "SourceTargetMode"
)

type DbSpec struct {
	// +required
	DbHost string `json:"dbHost"`
	// +optional
	// +default="3306"
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
	Path string `json:"path"`
	// +required
	S3Auth string `json:"s3Auth"`
}

type NewTargetModeSpec struct {
	// +required
	DbSpec DbSpec `json:"dbSpec"`
}

type SourceTargetModeSpec struct {
	// +required
	BakDBName string `json:"bakDBName,omitzero"`
}

type RestoreModeSpec struct {
	// +required
	ResotreMode ResoterMode `json:"restoreMode"`
	// +optional
	NewTargetModeSpec *NewTargetModeSpec `json:"newTargetModeSpec,omitzero"`
	// +optional
	SourceTargetModeSpec *SourceTargetModeSpec `json:"sourceTargetModeSpec,omitzero"`
}

type MySQLRestoreSpec struct {
	// +required
	DbSpec DbSpec `json:"dbSpec"`
	// +required
	S3Spec S3Spec `json:"s3Spec"`
	// +required
	ResotreModeSpec RestoreModeSpec `json:"restoreModeSpec"`
}

type LastRestoreSpec struct {
	// +requird
	DbHost string `json:"dbHost"`
	// +requird
	DbName string `json:"dbName"`
}

type MySQLRestoreStatus struct {
	// +required
	Conditions []metav1.Condition `json:"conditions,omitzero" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	// +optional
	LastRestoreSpec LastRestoreSpec `json:"lastRestoreSpec,omitzero"`
	// +optional
	LastStartTime metav1.Time `json:"lastStartTime,omitzero"`
	// +optional
	LastSucceedTime metav1.Time `json:"lastSucceedTime,omitzero"`
	// +optional
	LastFailedTime metav1.Time `json:"lastSailedTime,omitzero"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type MySQLRestore struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`
	// +required
	Spec MySQLRestoreSpec `json:"spec"`
	// +optional
	Status MySQLRestoreStatus `json:"status"`
}

// +kubebuilder:object:root=true
type MySQLRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []MySQLRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MySQLRestore{}, &MySQLRestoreList{})
}
