package controller

import (
	"context"
	"fmt"
	controllerapi "kserve-controller/api/v1"
	"kserve-controller/internal/helpers/kube/client"
	"os"

	v1batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"k8s.io/utils/ptr"

	ctrl "sigs.k8s.io/controller-runtime"

	finopsdatatypes "github.com/krateoplatformops/finops-data-types/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getJob(name string, namespace string) (*v1batch.Job, error) {
	config := ctrl.GetConfigOrDie()
	if config == nil {
		return nil, fmt.Errorf("could not get rest config")
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	jobClient := clientset.BatchV1().Jobs(namespace)
	job, err := jobClient.Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	} else {
		return job, nil
	}
}

func createJob(jobName string, iRun *controllerapi.InferenceRun, iConf *controllerapi.InferenceConfig) error {
	config := ctrl.GetConfigOrDie()
	if config == nil {
		return fmt.Errorf("could not get rest config")
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	jobClient := clientset.BatchV1().Jobs(iRun.Namespace)
	ttlTime := ptr.To(int32(300))
	if iConf.Spec.AutoDeletePolicy != nil {
		if *iConf.Spec.AutoDeletePolicy == controllerapi.AutoDeletePolicyNone {
			ttlTime = nil
		}
	}
	job := &v1batch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: jobName,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(iRun, controllerapi.GroupVersion.WithKind("InferenceRun")),
			},
		},
		Spec: v1batch.JobSpec{
			Completions:             ptr.To(int32(1)),
			TTLSecondsAfterFinished: ttlTime,
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:            "inference",
							ImagePullPolicy: v1.PullAlways,
							Image:           iConf.Spec.Image,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "contract",
									MountPath: "/tmp",
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "contract",
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: jobName,
									},
								},
							},
						},
					},
					RestartPolicy:      v1.RestartPolicyNever,
					ServiceAccountName: os.Getenv("SA_RUNNER"),
				},
			},
		},
	}
	if iRun.Spec.TimeoutSeconds != 0 {
		job.Spec.ActiveDeadlineSeconds = ptr.To(int64(iRun.Spec.TimeoutSeconds))
	}
	if iConf.Spec.CredentialsRef != nil {
		job.Spec.Template.Spec.ImagePullSecrets = []v1.LocalObjectReference{
			{
				Name: iConf.Spec.CredentialsRef.Name,
			},
		}
	}
	_, err = jobClient.Create(context.TODO(), job, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func updateStatus(ctx context.Context, iRun *controllerapi.InferenceRun) error {
	config := ctrl.GetConfigOrDie()
	if config == nil {
		return fmt.Errorf("could not get rest config")
	}
	dynClient, err := client.New(config)
	if err != nil {
		return fmt.Errorf("could not create dynamic client: %w", err)
	}

	unstructuredIRun, err := client.ToUnstructured(iRun)
	if err != nil {
		return fmt.Errorf("could not convert InferenceRun to unstructured: %w", err)
	}

	err = client.UpdateObj(ctx, unstructuredIRun, "inferenceruns", dynClient)
	if err != nil {
		return fmt.Errorf("could not update InferenceRun status: %w", err)
	}
	return nil
}

func deleteJob(iRun *controllerapi.InferenceRun, jobName string, propagate bool) error {
	config := ctrl.GetConfigOrDie()
	if config == nil {
		return fmt.Errorf("could not get rest config")
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	jobClient := clientset.BatchV1().Jobs(iRun.Namespace)
	deleteOptions := metav1.DeleteOptions{}
	if propagate {
		deleteOptions = metav1.DeleteOptions{PropagationPolicy: ptr.To(metav1.DeletePropagationBackground)}
	}
	err = jobClient.Delete(context.TODO(), jobName, deleteOptions)
	if err != nil {
		return err
	} else {
		return nil
	}
}

func createOrUpdateConfigMap(ctx context.Context, configmapName string, namespace string, contract []byte, iRun *controllerapi.InferenceRun) error {
	binaryData := make(map[string][]byte)
	binaryData["contract.json"] = contract
	configmap := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configmapName,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(iRun, controllerapi.GroupVersion.WithKind("InferenceRun")),
			},
		},
		BinaryData: binaryData,
	}

	config := ctrl.GetConfigOrDie()
	if config == nil {
		return fmt.Errorf("could not get rest config")
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	cmClient := clientset.CoreV1().ConfigMaps(namespace)
	_, err = cmClient.Create(ctx, configmap, metav1.CreateOptions{})
	if err != nil {
		_, errr := cmClient.Update(ctx, configmap, metav1.UpdateOptions{})
		if errr != nil {
			return fmt.Errorf("error creating configmap: %v, error updating configmap: %v", err, errr)
		}
	}

	return nil
}

func deleteRun(ctx context.Context, iRun *controllerapi.InferenceRun) error {
	config := ctrl.GetConfigOrDie()
	if config == nil {
		return fmt.Errorf("could not get rest config")
	}
	dynClient, err := client.New(config)
	if err != nil {
		return fmt.Errorf("could not create dynamic client: %w", err)
	}

	ref := &finopsdatatypes.ObjectRef{
		Name:      iRun.Name,
		Namespace: iRun.Namespace,
	}

	err = client.DeleteObj(ctx, ref, "ai.krateo.io/v1", "inferenceruns", dynClient)
	if err != nil {
		return fmt.Errorf("could not delete InferenceRun status: %w", err)
	}
	return nil
}
