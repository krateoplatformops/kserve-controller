package test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	v1batch "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/go-logr/logr"
	prettylog "github.com/krateoplatformops/plumbing/slogs/pretty"

	"github.com/krateoplatformops/provider-runtime/pkg/controller"
	"github.com/krateoplatformops/provider-runtime/pkg/logging"
	"github.com/krateoplatformops/provider-runtime/pkg/ratelimiter"

	controllerapi "kserve-controller/api/v1"
	kservecontroller "kserve-controller/internal/controller"
	"kserve-controller/internal/helpers/config"
	"kserve-controller/internal/helpers/job"

	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/support/kind"
)

type contextKey string

var (
	testenv   env.Environment
	testnames = map[string]string{
		"testdata-custom":          "{\"jobId\":\"\",\"jobName\":\"\",\"kserve\":{\"modelName\":\"sklearn-iris\",\"modelUrl\":\"kserve-krateo-ttm-predictor.kserve-test.svc.cluster.local/v2/models/granite-timeseries-ttm-r2/infer\",\"modelVersion\":\"v2\",\"modelInputName\":\"past_values\"},\"input\":{\"krateo\":{\"api\":{\"endpointRef\":{\"name\":\"finops-database-handler-endpoint\",\"namespace\":\"kserve-controller-system\"},\"headers\":[\"Accept: application/json\",\"Content-Type: application/json\"],\"path\":\"/compute/tritoninput\",\"verb\":\"POST\"}}},\"output\":{\"custom\":{\"s3\":{\"endpointRef\":{\"name\":\"finops-database-handler-endpoint\",\"namespace\":\"kserve-controller-system\"},\"moreCustom\":\"data\"}}},\"parameters\":{\"input_table_name\":\"azuretoolkit\",\"output_table_name\":\"kserve_controller_output_triton\"}}",
		"testdata-iris-autodelete": "{\"jobId\":\"\",\"jobName\":\"\",\"kserve\":{\"modelName\":\"sklearn-iris\",\"modelUrl\":\"sklearn-iris-predictor.kserve-test.svc.cluster.local/v2/models/sklearn-iris/infer\",\"modelVersion\":\"v2\",\"modelInputName\":\"input-0\"},\"input\":{\"krateo\":{\"api\":{\"endpointRef\":{\"name\":\"finops-database-handler-endpoint\",\"namespace\":\"kserve-controller-system\"},\"headers\":[\"Accept: application/json\",\"Content-Type: application/json\"],\"path\":\"/compute/sklearninput\",\"verb\":\"POST\"}}},\"output\":{\"krateo\":{\"api\":{\"endpointRef\":{\"name\":\"finops-database-handler-endpoint\",\"namespace\":\"kserve-controller-system\"},\"headers\":[\"Accept: application/json\",\"Content-Type: application/json\"],\"path\":\"/compute/sklearnoutput\",\"verb\":\"POST\"}}},\"parameters\":{\"input_table_name\":\"kserve_controller_input_sklearn\",\"output_table_name\":\"kserve_controller_output_sklearn\"}}",
		"testdata-iris-schedule":   "{\"jobId\":\"\",\"jobName\":\"\",\"kserve\":{\"modelName\":\"sklearn-iris\",\"modelUrl\":\"sklearn-iris-predictor.kserve-test.svc.cluster.local/v2/models/sklearn-iris/infer\",\"modelVersion\":\"v2\",\"modelInputName\":\"input-0\"},\"input\":{\"krateo\":{\"api\":{\"endpointRef\":{\"name\":\"finops-database-handler-endpoint\",\"namespace\":\"kserve-controller-system\"},\"headers\":[\"Accept: application/json\",\"Content-Type: application/json\"],\"path\":\"/compute/sklearninput\",\"verb\":\"POST\"}}},\"output\":{\"krateo\":{\"api\":{\"endpointRef\":{\"name\":\"finops-database-handler-endpoint\",\"namespace\":\"kserve-controller-system\"},\"headers\":[\"Accept: application/json\",\"Content-Type: application/json\"],\"path\":\"/compute/sklearnoutput\",\"verb\":\"POST\"}}},\"parameters\":{\"input_table_name\":\"kserve_controller_input_sklearn\",\"output_table_name\":\"kserve_controller_output_sklearn\"}}",
		"testdata-triton":          "{\"jobId\":\"\",\"jobName\":\"\",\"kserve\":{\"modelName\":\"sklearn-iris\",\"modelUrl\":\"kserve-krateo-ttm-predictor.kserve-test.svc.cluster.local/v2/models/granite-timeseries-ttm-r2/infer\",\"modelVersion\":\"v2\",\"modelInputName\":\"past_values\"},\"input\":{\"krateo\":{\"api\":{\"endpointRef\":{\"name\":\"finops-database-handler-endpoint\",\"namespace\":\"kserve-controller-system\"},\"headers\":[\"Accept: application/json\",\"Content-Type: application/json\"],\"path\":\"/compute/tritoninput\",\"verb\":\"POST\"}}},\"output\":{\"krateo\":{\"api\":{\"endpointRef\":{\"name\":\"finops-database-handler-endpoint\",\"namespace\":\"kserve-controller-system\"},\"headers\":[\"Accept: application/json\",\"Content-Type: application/json\"],\"path\":\"/compute/tritonoutput\",\"verb\":\"POST\"}}},\"parameters\":{\"input_data_length\":\"512\",\"input_table_column_name\":\"average\",\"input_table_name\":\"azuretoolkit\",\"key_name\":\"resourceid\",\"key_value\":\"sample-vm\",\"output_table_name\":\"kserve_controller_output_triton\"}}",
	}
)

const (
	testNamespace   = "kserve-test"
	krateoNamespace = "krateo-system"
	helmPath        = "../../chart"
	crdsPath1       = "../../crds"
	crdsPath2       = "./crds"
	deploymentsPath = "./deployments"
	toTest          = "../../testdata"
)

func TestMain(m *testing.M) {
	testenv = env.New()
	kindClusterName := "krateo-test"

	testenv.Setup(
		envfuncs.CreateCluster(kind.NewProvider(), kindClusterName),
		envfuncs.CreateNamespace(testNamespace),
		envfuncs.CreateNamespace(krateoNamespace),
		envfuncs.SetupCRDs(crdsPath1, "*"),
		envfuncs.SetupCRDs(crdsPath2, "*"),
	)

	testenv.Finish(
		envfuncs.DeleteNamespace(testNamespace),
		envfuncs.TeardownCRDs(crdsPath1, "*"),
		envfuncs.DestroyCluster(kindClusterName),
	)

	os.Exit(testenv.Run(m))
}

func TestController(t *testing.T) {
	mgrCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	createAndDelete := features.New("Create").
		WithLabel("type", "CR and resources").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			r, err := resources.New(c.Client().RESTConfig())
			if err != nil {
				t.Fatal(err)
			}

			ctx = context.WithValue(ctx, contextKey("client"), r)

			controllerapi.AddToScheme(r.GetScheme())
			r.WithNamespace(testNamespace)

			// Start the controller manager
			err = startTestManager(mgrCtx, r.GetScheme())
			if err != nil {
				t.Fatal(err)
			}

			err = decoder.DecodeEachFile(
				ctx, os.DirFS(deploymentsPath), "*",
				decoder.CreateHandler(r),
				decoder.MutateNamespace(testNamespace),
			)
			if err != nil {
				t.Fatalf("Failed due to error: %s", err)
			}

			// Create test resources
			err = decoder.DecodeEachFile(
				ctx, os.DirFS(toTest), "*",
				decoder.CreateHandler(r),
				decoder.MutateNamespace(testNamespace),
			)
			if err != nil {
				t.Fatal(err)
			}

			return ctx
		}).Assess("Verify Resources", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		r := ctx.Value(contextKey("client")).(*resources.Resources)

		for name := range testnames {
			// 1. Get the InferenceRun to extract the JobName from Status
			cr := &controllerapi.InferenceRun{}
			err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
				if err := r.Get(ctx, name, testNamespace, cr); err != nil {
					return false, nil
				}
				return len(cr.Status.Contract) > 0, nil
			})
			if err != nil {
				t.Fatalf("Failed to get InferenceRun %s: %v", name, err)
			}

			// 2. Parse the contract to get the dynamic JobName
			contractData := &job.ContractSpec{}
			if err := json.Unmarshal(cr.Status.Contract, contractData); err != nil {
				t.Fatalf("Failed to unmarshal contract for %s: %v", name, err)
			}

			generatedName := contractData.JobName
			if generatedName == "" {
				t.Fatalf("JobName is empty in contract for %s", name)
			}

			// 3. Verify the background resource (Job or CronJob)
			// Logic: if the test name contains "schedule", look for a CronJob. Otherwise, a Job.
			isSchedule := (name == "testdata-iris-schedule")

			if isSchedule {
				cj := &v1batch.CronJob{}
				err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
					err := r.Get(ctx, generatedName, testNamespace, cj)
					return err == nil, nil
				})
				if err != nil {
					t.Errorf("CronJob %s not found for InferenceRun %s: %v", generatedName, name, err)
				} else {
					t.Logf("Verified CronJob %s exists", generatedName)
				}
			} else {
				jb := &v1batch.Job{}
				err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
					err := r.Get(ctx, generatedName, testNamespace, jb)
					return err == nil, nil
				})
				if err != nil {
					t.Errorf("Job %s not found for InferenceRun %s: %v", generatedName, name, err)
				} else {
					t.Logf("Verified Job %s exists", generatedName)
				}
			}
		}
		return ctx
	}).Assess("Delete and Cleanup", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		r := ctx.Value(contextKey("client")).(*resources.Resources)

		time.Sleep(50 * time.Second)

		for name := range testnames {
			cr := &controllerapi.InferenceRun{}
			if err := r.Get(ctx, name, testNamespace, cr); err != nil {
				t.Errorf("Could not find CR %s for deletion: %v", name, err)
				continue
			}

			// Extract name for cleanup check before deleting
			contractData := &job.ContractSpec{}
			json.Unmarshal(cr.Status.Contract, contractData)
			generatedName := contractData.JobName

			// 1. Delete the CR
			if err := r.Delete(ctx, cr); err != nil {
				t.Errorf("Failed to delete CR %s: %v", name, err)
				continue
			}

			// 2. Poll for cleanup of Job/CronJob and ConfigMap
			err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
				cm := &v1.ConfigMap{}
				cmErr := r.Get(ctx, generatedName, testNamespace, cm)

				var resErr error
				if name == "testdata-iris-schedule" {
					resErr = r.Get(ctx, generatedName, testNamespace, &v1batch.CronJob{})
				} else {
					resErr = r.Get(ctx, generatedName, testNamespace, &v1batch.Job{})
				}

				// Return true if both are gone (IsNotFound)
				return errors.IsNotFound(cmErr) && errors.IsNotFound(resErr), nil
			})

			if err != nil {
				t.Errorf("Cleanup timed out for %s: child resources still exist", name)
			}
		}
		return ctx
	}).Feature()

	// Run both features
	testenv.Test(t, createAndDelete)
}

// startTestManager starts the controller manager with the given config
func startTestManager(ctx context.Context, scheme *runtime.Scheme) error {
	setupLog := ctrl.Log.WithName("setup")
	os.Setenv("WATCH_NAMESPACE", testNamespace)
	os.Setenv("POLLING_INTERVAL", "10")

	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	configuration := config.ParseConfig()

	lh := prettylog.New(&slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: false,
	},
		prettylog.WithDestinationWriter(os.Stderr),
		prettylog.WithColor(),
		prettylog.WithOutputEmptyAttrs(),
	)

	logrlog := logr.FromSlogHandler(slog.New(lh).Handler())
	log := logging.NewLogrLogger(logrlog)

	// Set the logger for controller-runtime. This only have to log in INFO level as all debug logs are handled by our logger above.
	ctrl.SetLogger(logr.FromSlogHandler(slog.New(prettylog.New(&slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: false,
	},
		prettylog.WithDestinationWriter(os.Stderr),
		prettylog.WithColor(),
		prettylog.WithOutputEmptyAttrs(),
	)).Handler()))

	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	watchNamespace := configuration.WatchNamespace
	namespaceCacheConfigMap := make(map[string]cache.Config)
	namespaceCacheConfigMap[watchNamespace] = cache.Config{}
	if watchNamespace == "" {
		setupLog.Error(fmt.Errorf("WATCH_NAMESPACE is empty"), "unable to get WatchNamespace")
		os.Exit(1)
	}

	pollingIntervalString := configuration.PollingInterval
	maxReconcileRateString := configuration.MaxReconcileRate

	pollingIntervalInt, err := strconv.Atoi(pollingIntervalString)
	pollingInterval := time.Duration(time.Duration(0))

	if err != nil {
		setupLog.Error(err, "unable to parse POLLING_INTERVAL, using default value")
	} else if pollingIntervalInt != 0 {
		pollingInterval = time.Duration(pollingIntervalInt) * time.Second
	}

	maxReconcileRate, err := strconv.Atoi(maxReconcileRateString)
	if err != nil {
		setupLog.Error(err, "unable to parse MAX_RECONCILE_RATE, using default value (1)")
		maxReconcileRate = 1
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "9s8d9938.krateo.io",
		Cache:                  cache.Options{DefaultNamespaces: namespaceCacheConfigMap},
	})
	if err != nil {
		os.Exit(1)
	}

	o := controller.Options{
		Logger:                  log,
		MaxConcurrentReconciles: maxReconcileRate,
		PollInterval:            pollingInterval,
		GlobalRateLimiter:       ratelimiter.NewGlobal(maxReconcileRate),
	}

	if err := kservecontroller.Setup(mgr, o, configuration); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CompositionReference")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	go func() {
		if err := mgr.Start(ctx); err != nil { // Use the passed ctx, not SetupSignalHandler
			setupLog.Error(err, "problem running manager")
		}
	}()
	return nil
}
