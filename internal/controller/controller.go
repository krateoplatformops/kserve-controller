package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/krateoplatformops/provider-runtime/pkg/controller"
	"github.com/krateoplatformops/provider-runtime/pkg/event"
	"github.com/krateoplatformops/provider-runtime/pkg/logging"
	"github.com/krateoplatformops/provider-runtime/pkg/ratelimiter"
	"github.com/krateoplatformops/provider-runtime/pkg/reconciler"
	"github.com/krateoplatformops/provider-runtime/pkg/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"

	prv1 "github.com/krateoplatformops/provider-runtime/apis/common/v1"

	controllerapi "kserve-controller/api/v1"
	"kserve-controller/internal/helpers"
	"kserve-controller/internal/helpers/config"
	"kserve-controller/internal/helpers/job"
	clientHelper "kserve-controller/internal/helpers/kube/client"
)

type JobStatus string

const (
	JobStatusPending   JobStatus = "Pending"
	JobStatusRunning   JobStatus = "Running"
	JobStatusSucceeded JobStatus = "Succeeded"
	JobStatusFailed    JobStatus = "Failed"
	JobStatusUnknown   JobStatus = "Unknown"

	JOB_NAME_PREFIX string = "inf"
)

func Setup(mgr ctrl.Manager, o controller.Options, config config.Configuration) error {
	name := reconciler.ControllerName(controllerapi.GroupKind)

	log := o.Logger.WithValues("controller", name)
	log.Info("controller", "name", name)

	recorder := mgr.GetEventRecorderFor(name)

	r := reconciler.NewReconciler(mgr,
		resource.ManagedKind(controllerapi.GroupVersionKind),
		reconciler.WithExternalConnecter(&connector{
			log:          log,
			recorder:     recorder,
			pollInterval: o.PollInterval,
			config:       config,
		}),
		reconciler.WithPollInterval(o.PollInterval),
		reconciler.WithLogger(log),
		reconciler.WithRecorder(event.NewAPIRecorder(recorder)))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&controllerapi.InferenceRun{}).
		Complete(ratelimiter.New(name, r, o.GlobalRateLimiter))
}

type connector struct {
	pollInterval time.Duration
	log          logging.Logger
	recorder     record.EventRecorder
	config       config.Configuration
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (reconciler.ExternalClient, error) {
	cfg := ctrl.GetConfigOrDie()

	dynClient, err := clientHelper.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to create dynamic client: %w", err)
	}

	return &external{
		cfg:          cfg,
		dynClient:    dynClient,
		pollInterval: c.pollInterval,
		log:          c.log,
		rec:          c.recorder,
		config:       c.config,
	}, nil
}

type external struct {
	cfg          *rest.Config
	dynClient    *dynamic.DynamicClient
	pollInterval time.Duration
	log          logging.Logger
	rec          record.EventRecorder
	config       config.Configuration
}

func (c *external) Disconnect(_ context.Context) error {
	return nil // NOOP
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (reconciler.ExternalObservation, error) {
	iRun, ok := mg.(*controllerapi.InferenceRun)
	if !ok {
		return reconciler.ExternalObservation{}, fmt.Errorf("cannot cast to controllerapi.InferenceRun")
	}

	log := e.log.WithValues("Reconcile", "Observe", "name", iRun.Name, "namespace", iRun.Namespace)

	iConf, err := getIConf(ctx, iRun, e.dynClient)
	if err != nil {
		log.Warn(fmt.Sprintf("unable to retrieve InferenceConfig referenced in InferenceRun: %v", err))
		return reconciler.ExternalObservation{
			ResourceExists: false,
		}, nil
	}
	log.Info(fmt.Sprintf("retrieved InferenceConfig %s", iConf.Name))

	jobName := helpers.ComputeJobName(JOB_NAME_PREFIX, iRun.Name, string(iRun.UID))

	contract := job.ContractSpec{
		JobId:      string(iRun.UID),
		JobName:    jobName,
		KServe:     iConf.Spec.KServe,
		Input:      iConf.Spec.Storage.Input,
		Output:     iConf.Spec.Storage.Output,
		Parameters: iRun.Spec.Parameters,
	}

	contractJson, err := json.Marshal(contract)
	if err != nil {
		return reconciler.ExternalObservation{}, fmt.Errorf("unable to marshal contract to json: %w", err)
	}

	log.Debug(fmt.Sprintf("observe: InferenceRun %s computed contract", iRun.Name))
	log.Debug("Contract: " + string(contractJson))

	job, err := getJob(contract.JobName, iRun.Namespace)
	if err != nil {
		log.Warn(fmt.Sprintf("unable to retrieve job: %v", err))
	}

	if job != nil {
		iRun.Status.JobStatus = &job.Status
	} else {
		iRun.Status.JobStatus = nil
	}
	iRun.Status.Contract = contractJson
	err = updateStatus(ctx, iRun)
	if err != nil {
		return reconciler.ExternalObservation{}, fmt.Errorf("unable to update InferenceRun status: %w", err)
	}

	err = createOrUpdateConfigMap(ctx, jobName, iRun.Namespace, iRun.Status.Contract, iRun)
	if err != nil {
		return reconciler.ExternalObservation{}, fmt.Errorf("unable to create configmap for job %s: %w", jobName, err)
	}

	log.Info(fmt.Sprintf("created configmap for job %s with contract %s", jobName, string(iRun.Status.Contract)))

	if iRun.Status.JobStatus == nil {
		log.Info(fmt.Sprintf("%s does not have a job yet", iRun.Name))

		return reconciler.ExternalObservation{
			ResourceExists: false,
		}, nil
	} else if status := computeJobStatus(iRun); status != "" {
		log.Info(fmt.Sprintf("%s exists with status %s", jobName, string(status)))
		switch status {
		case JobStatusFailed:
			errorMessage := ""
			if len(iRun.Status.JobStatus.Conditions) > 0 {
				for _, cond := range iRun.Status.JobStatus.Conditions {
					if cond.Status == "False" {
						errorMessage += cond.Message + "; "
					}
				}
			}
			if errorMessage == "" {
				errorMessage = "unknown error"
			}
			log.Warn(fmt.Sprintf("inference job failed: %s", errorMessage))
			return reconciler.ExternalObservation{
				ResourceExists:   true,
				ResourceUpToDate: false,
			}, nil
		case JobStatusSucceeded:
			if iRun.Status.JobStatus != nil {
				log.Info(fmt.Sprintf("checking autoDeletePolicy for InferenceRun %s", iRun.Name))
				if autoDeletePolicy(iConf, computeJobStatus(iRun)) {
					log.Info(fmt.Sprintf("deleting InferenceRun %s for AutoDeletePolicy", iRun.Name))
					err := deleteRun(ctx, iRun)
					if err != nil {
						return reconciler.ExternalObservation{}, fmt.Errorf("unable to delete InferenceRun %s: %w", iRun.Name, err)
					}
				}
			}
			// These are reported explicitly, but commented since they are the same and covered outside the if
			// 	iRun.SetConditions(prv1.Available())
			// 	return reconciler.ExternalObservation{
			// 		ResourceExists:   true,
			// 		ResourceUpToDate: true,
			// 	}, nil
			// case JobStatusRunning:
			// 	iRun.SetConditions(prv1.Available())
			// 	return reconciler.ExternalObservation{
			// 		ResourceExists:   true,
			// 		ResourceUpToDate: true,
			// 	}, nil
			// case JobStatusPending:
			// 	iRun.SetConditions(prv1.Available())
			// 	return reconciler.ExternalObservation{
			// 		ResourceExists:   true,
			// 		ResourceUpToDate: true,
			// 	}, nil
		}
	}

	iRun.SetConditions(prv1.Available())
	return reconciler.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: true,
	}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) error {
	iRun, ok := mg.(*controllerapi.InferenceRun)
	if !ok {
		return fmt.Errorf("cannot cast to controllerapi.InferenceRun")
	}

	log := e.log.WithValues("Reconcile", "Create", "name", iRun.Name, "namespace", iRun.Namespace)

	iConf, err := getIConf(ctx, iRun, e.dynClient)
	if err != nil {
		return fmt.Errorf("unable to retrieve InferenceConfig referenced in InferenceRun: %w", err)
	}

	log.Info(fmt.Sprintf("retrieved InferenceConfig %s for %s", iConf.Name, iRun.Name))

	iRun.SetConditions(prv1.Creating())

	jobName := helpers.ComputeJobName(JOB_NAME_PREFIX, iRun.Name, string(iRun.UID))

	err = createJob(jobName, iRun, iConf)
	if err != nil {
		return fmt.Errorf("unable to create job: %w", err)
	}

	log.Info(fmt.Sprintf("created job %s for InferenceRun %s", jobName, iRun.Name))

	job, err := getJob(jobName, iRun.Namespace)
	if err != nil {
		log.Warn(fmt.Sprintf("unable to retrieve job: %v", err))
	}

	if job != nil {
		iRun.Status.JobStatus = &job.Status
	} else {
		iRun.Status.JobStatus = nil
	}
	err = updateStatus(ctx, iRun)
	if err != nil {
		return fmt.Errorf("unable to update InferenceRun status: %w", err)
	}

	return nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) error {
	iRun, ok := mg.(*controllerapi.InferenceRun)
	if !ok {
		return fmt.Errorf("cannot cast to controllerapi.InferenceRun")
	}

	log := e.log.WithValues("Reconcile", "Update", "name", iRun.Name, "namespace", iRun.Namespace)

	iConf, err := getIConf(ctx, iRun, e.dynClient)
	if err != nil {
		return fmt.Errorf("unable to retrieve InferenceConfig referenced in InferenceRun: %w", err)
	}
	log.Info(fmt.Sprintf("retrieved InferenceConfig %s for %s", iConf.Name, iRun.Name))

	if iRun.Status.JobStatus != nil {
		log.Info(fmt.Sprintf("checking autoDeletePolicy for InferenceRun %s", iRun.Name))
		if autoDeletePolicy(iConf, computeJobStatus(iRun)) {
			log.Info(fmt.Sprintf("deleting InferenceRun %s for AutoDeletePolicy", iRun.Name))
			err := deleteRun(ctx, iRun)
			if err != nil {
				return fmt.Errorf("unable to delete job for InferenceRun %s: %w", iRun.Name, err)
			}
		} else {
			// Re-create the job to restart it
			err := deleteJob(iRun, helpers.ComputeJobName(JOB_NAME_PREFIX, iRun.Name, string(iRun.UID)), true)
			if err != nil {
				return fmt.Errorf("unable to delete job for InferenceRun %s: %w", iRun.Name, err)
			}
		}
	}
	return nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) error {
	iRun, ok := mg.(*controllerapi.InferenceRun)
	if !ok {
		return fmt.Errorf("cannot cast to controllerapi.InferenceRun")
	}

	log := e.log.WithValues("Reconcile", "Delete", "name", iRun.Name, "namespace", iRun.Namespace)

	iConf, err := getIConf(ctx, iRun, e.dynClient)
	if err != nil {
		return err
	}
	log.Info(fmt.Sprintf("retrieved InferenceConfig %s", iConf.Name))

	iRun.SetConditions(prv1.Deleting())

	jobName := helpers.ComputeJobName(JOB_NAME_PREFIX, iRun.Name, string(iRun.UID))

	log.Info(fmt.Sprintf("receive delete for %s, deleting job %s", iRun.Name, jobName))

	if iRun.Status.JobStatus != nil {
		log.Info(fmt.Sprintf("checking autoDeletePolicy for InferenceRun %s", iRun.Name))
		if autoDeletePolicy(iConf, computeJobStatus(iRun)) {
			err = deleteJob(iRun, jobName, true)
		} else {
			err = deleteJob(iRun, jobName, false)
		}
	} else {
		log.Warn(fmt.Sprintf("JobStatus for InferenceRun %s not available, propagation to pods for job deletion disabled", iRun.Name))
		err = deleteJob(iRun, jobName, false)
	}
	if err != nil {
		return fmt.Errorf("unable to delete job %s: %w", jobName, err)
	}

	return nil
}

func computeJobStatus(iRun *controllerapi.InferenceRun) JobStatus {
	if iRun.Status.JobStatus.Active == 0 && iRun.Status.JobStatus.Succeeded == 0 && iRun.Status.JobStatus.Failed == 0 {
		return JobStatusPending
	} else if iRun.Status.JobStatus.Active > 0 {
		return JobStatusRunning
	} else if iRun.Status.JobStatus.Succeeded > 0 {
		return JobStatusSucceeded
	} else if iRun.Status.JobStatus.Failed > 0 {
		return JobStatusFailed
	}
	return JobStatusUnknown
}

func getIConf(ctx context.Context, iRun *controllerapi.InferenceRun, dynClient *dynamic.DynamicClient) (*controllerapi.InferenceConfig, error) {
	iConfUn, err := clientHelper.GetObj(ctx, iRun.Spec.ConfigRef, "ai.krateo.io/v1", "inferenceconfigs", dynClient)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve InferenceConfig referenced in InferenceRun: %w", err)
	}

	iConf := &controllerapi.InferenceConfig{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(iConfUn.Object, iConf)
	if err != nil {
		return nil, fmt.Errorf("unable to convert InferenceConfig from unstructured: %w", err)
	}
	return iConf, nil
}

func autoDeletePolicy(iConf *controllerapi.InferenceConfig, status JobStatus) bool {
	switch status {
	case JobStatusSucceeded:
		if iConf.Spec.AutoDeletePolicy != nil {
			switch *iConf.Spec.AutoDeletePolicy {
			case controllerapi.AutoDeletePolicyNone:
				return false
			case controllerapi.AutoDeletePolicyDeleteOnCompletion:
				return true
			case controllerapi.AutoDeletePolicyDeleteOnSuccess:
				return true
			}
		}
	case JobStatusFailed:
		if iConf.Spec.AutoDeletePolicy != nil {
			switch *iConf.Spec.AutoDeletePolicy {
			case controllerapi.AutoDeletePolicyNone:
				return false
			case controllerapi.AutoDeletePolicyDeleteOnCompletion:
				return true
			case controllerapi.AutoDeletePolicyDeleteOnSuccess:
				return false
			}
		}
	}
	return false
}
