// +kubebuilder:object:generate=true
package v1

import (
	finopsdatatypes "github.com/krateoplatformops/finops-data-types/api/v1"
	prv1 "github.com/krateoplatformops/provider-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type InferenceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InferenceConfigSpec   `json:"spec,omitempty"`
	Status InferenceConfigStatus `json:"status,omitempty"`
}

type AutoDeletePolicy string

const (
	AutoDeletePolicyNone               AutoDeletePolicy = "None"
	AutoDeletePolicyDeleteOnCompletion AutoDeletePolicy = "DeleteOnCompletion"
	AutoDeletePolicyDeleteOnSuccess    AutoDeletePolicy = "DeleteOnSuccess"
)

type InferenceConfigSpec struct {
	KServe KServeSpec `json:"kserve,omitempty"`
	// +kubebuilder:validation:Enum=None;DeleteOnSuccess;DeleteOnCompletion
	AutoDeletePolicy *AutoDeletePolicy          `json:"autoDeletePolicy"`
	Storage          StorageSpec                `json:"storage"`
	Image            string                     `json:"image"`
	CredentialsRef   *finopsdatatypes.ObjectRef `json:"credentialsRef,omitempty"`
}

type KServeSpec struct {
	ModelName      string `json:"modelName,omitempty"`
	ModelUrl       string `json:"modelUrl,omitempty"`
	ModelVersion   string `json:"modelVersion,omitempty"`
	ModelInputName string `json:"modelInputName,omitempty"`
}

type StorageSpec struct {
	Input  StorageMap `json:"input,omitempty"`
	Output StorageMap `json:"output,omitempty"`
}

type InferenceConfigStatus struct {
	prv1.ConditionedStatus `json:",inline"`
}

//+kubebuilder:object:root=true

type InferenceConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InferenceConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InferenceConfig{}, &InferenceConfigList{})
}

func (mg *InferenceConfig) GetCondition(ct prv1.ConditionType) prv1.Condition {
	return mg.Status.GetCondition(ct)
}

func (mg *InferenceConfig) SetConditions(c ...prv1.Condition) {
	mg.Status.SetConditions(c...)
}
