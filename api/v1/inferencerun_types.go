// +kubebuilder:object:generate=true
package v1

import (
	finopsdatatypes "github.com/krateoplatformops/finops-data-types/api/v1"
	prv1 "github.com/krateoplatformops/provider-runtime/apis/common/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type InferenceRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InferenceRunSpec   `json:"spec,omitempty"`
	Status InferenceRunStatus `json:"status,omitempty"`
}

type InferenceRunSpec struct {
	ConfigRef      *finopsdatatypes.ObjectRef `json:"configRef"`
	TimeoutSeconds int                        `json:"timeoutSeconds"`
	Parameters     *map[string]string         `json:"parameters,omitempty"`
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="(@(annually|yearly|monthly|weekly|daily|midnight|hourly))|((((\\d+,)+\\d+|(\\d+(\\/|-)\\d+)|\\d+|\\*) ?){5,7})"
	Schedule *string `json:"schedule,omitempty"`
}

type InferenceRunStatus struct {
	prv1.ConditionedStatus `json:",inline"`
	Contract               []byte              `json:"contract,omitempty"`
	JobStatus              *v1.ObjectReference `json:"jobStatus,omitempty"`
}

//+kubebuilder:object:root=true

type InferenceRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InferenceRun `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InferenceRun{}, &InferenceRunList{})
}

func (mg *InferenceRun) GetCondition(ct prv1.ConditionType) prv1.Condition {
	return mg.Status.GetCondition(ct)
}

func (mg *InferenceRun) SetConditions(c ...prv1.Condition) {
	mg.Status.SetConditions(c...)
}
