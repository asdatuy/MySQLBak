package controller

import (
	"context"
	"fmt"
	dbrestorev1alpha1 "local/MySQLRestore/api/v1alpha1"
	"reflect"
	"sort"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	jobOwnerKey = ".metadata.controller"
	apiGvr      = dbrestorev1alpha1.GroupVersion.String()
)

// MySQLRestoreReconciler reconciles a MySQLRestore object
type MySQLRestoreReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// 我们需要对Job有相关的权限: get;create;list;delete
// +kubebuilder:rbac:groups=dbrestore.local.com,resources=mysqlrestores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbrestore.local.com,resources=mysqlrestores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbrestore.local.com,resources=mysqlrestores/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
func (r *MySQLRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	instance := &dbrestorev1alpha1.MySQLRestore{}

	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("No aviliable CR instances")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Not Fetch CRD")
		return ctrl.Result{}, err
	}
	// initStatus
	// rStatusUpdate := func(phase string) (ctrl.Result, error) {
	// 	if !reflect.DeepEqual(statuRep, instance.Status) {
	// 		instance.Status = *statuRep.DeepCopy()
	// 		if err := r.Status().Update(ctx, instance); err != nil {
	// 			logger.Error(err, "Update status failed", "Phase:", phase)
	// 		}
	// 		return ctrl.Result{}, nil
	// 	}
	// 	logger.Info("No change")
	// 	return ctrl.Result{}, nil
	// }
	if len(instance.Status.Conditions) == 0 {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               "unkonw",
			Status:             metav1.ConditionUnknown,
			Reason:             "initStatus",
			Message:            "FirstSetStatus",
			LastTransitionTime: metav1.NewTime(time.Now()),
		})
		instance.Status.LastStartTime = metav1.Time{Time: time.Unix(0, 0)}
		instance.Status.LastSucceedTime = metav1.Time{Time: time.Unix(0, 0)}
		instance.Status.LastFailedTime = metav1.Time{Time: time.Unix(0, 0)}
		instance.Status.LastRestoreSpec = dbrestorev1alpha1.LastRestoreSpec{}
		err := r.Status().Update(ctx, instance)
		if err != nil {
			logger.Info("[Init]StatusInitSucceed")
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	// list allJobs owner by CR
	var jobList batchv1.JobList
	if err := r.List(ctx, &jobList, client.InNamespace(req.Namespace), client.MatchingFields{jobOwnerKey: req.Name}); err != nil {
		logger.Error(err, "[List]Failed JobList")
		return ctrl.Result{}, err
	}

	crRep := instance.DeepCopy()
	// Create job if len(jobList) == 0
	if len(jobList.Items) == 0 {
		// Check Secrets aviliable
		chkSecretsList := []string{crRep.Spec.DbSpec.DbAuth, crRep.Spec.S3Spec.S3Auth}
		if crRep.Spec.ResotreModeSpec.ResotreMode == dbrestorev1alpha1.NewTargetMode {
			chkSecretsList = append(chkSecretsList, crRep.Spec.ResotreModeSpec.NewTargetModeSpec.DbSpec.DbAuth)
		}
		for _, v := range chkSecretsList {
			if err := r.checkSecrets(instance, ctx, v); err != nil {
				logger.Error(err, "[Secrets] check failed", "SecretsName", v)
				meta.SetStatusCondition(&crRep.Status.Conditions, metav1.Condition{
					Type:    "Check",
					Status:  metav1.ConditionTrue,
					Reason:  "SecretsCheckFailed",
					Message: fmt.Sprint("%V is unaviliale", v),
				})
				if !reflect.DeepEqual(crRep.Status, instance.Status) {
					if err := r.Status().Update(ctx, crRep); err != nil {
						logger.Error(err, "[Status] update FAILED(phase: CheckSecets)")
						return ctrl.Result{}, err
					}
				}
				return ctrl.Result{}, err
			}
		}

		// checkJobTmp
		jobTmp, err := r.BuildJobTmp(crRep)
		if err != nil {
			logger.Error(err, "[jobTmpBuildFailed]")
			meta.SetStatusCondition(&crRep.Status.Conditions, metav1.Condition{
				Type:    "jobTmpBuild",
				Status:  metav1.ConditionTrue,
				Reason:  "jobTmpBuildFailed",
				Message: "jobtemp build is failed",
			})
			if !reflect.DeepEqual(crRep.Status, instance.Status) {
				if err := r.Status().Update(ctx, instance); err != nil {
					logger.Error(err, "[Status]update FAILED(phase: CheckSecets)")
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{}, err
		}

		// buildJob from jobTmp
		if err = r.Create(ctx, jobTmp); err != nil {
			logger.Error(err, "[jobCreate]JobCreateFailed")
			meta.SetStatusCondition(&crRep.Status.Conditions, metav1.Condition{
				Type:    "jobCreate",
				Status:  metav1.ConditionTrue,
				Reason:  "jobCreateFailed",
				Message: "job create is failed",
			})
			if !reflect.DeepEqual(crRep.Status, instance.Status) {
				if err := r.Status().Update(ctx, crRep); err != nil {
					logger.Error(err, "[Status]update FAILED(phase: CheckSecets)")
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Sort JobList
	sort.Slice(jobList.Items, func(i int, j int) bool {
		return jobList.Items[i].CreationTimestamp.Before(&jobList.Items[j].CreationTimestamp)
	})

	for _, job := range jobList.Items {
		_, jobFinStatus := IsJobFin(&job)
		switch jobFinStatus {
		case "":
			{
				if crRep.Status.LastStartTime.Before(&job.CreationTimestamp) {
					logger.Info("Have new job ACTIVE", "jobName", job.CreationTimestamp, "LastStartTime", crRep.Status.LastStartTime)
					meta.SetStatusCondition(&crRep.Status.Conditions, metav1.Condition{
						Type:               "Running",
						Status:             metav1.ConditionTrue,
						Reason:             "JobRunning",
						Message:            "JobRunning",
						LastTransitionTime: metav1.NewTime(time.Now()),
					})
					crRep.Status.LastStartTime = job.CreationTimestamp
					if !reflect.DeepEqual(crRep.Status, instance.Status) {
						if err := r.Status().Update(ctx, crRep); err != nil {
							logger.Error(err, "Status update FAILED(phase: finalUpdate)")
							return ctrl.Result{}, err
						}
						return ctrl.Result{}, nil
					}
				}
			}
		case batchv1.JobComplete:
			{
				// JobIsCompleted
				if crRep.Status.LastSucceedTime.Before(job.Status.CompletionTime) {
					logger.Info("Have new job COMPLETED", "jobName", job.Name)
					meta.SetStatusCondition(&crRep.Status.Conditions, metav1.Condition{
						Type:               "Completed",
						Status:             metav1.ConditionTrue,
						Reason:             "JobCompleted",
						Message:            "Job is Completed",
						LastTransitionTime: metav1.NewTime(time.Now()),
					})
					lastBakSpec := dbrestorev1alpha1.LastRestoreSpec{
						DbHost: crRep.Spec.DbSpec.DbHost + ":" + crRep.Spec.DbSpec.DbPort,
						DbName: crRep.Spec.DbSpec.DbName,
					}
					crRep.Status.LastSucceedTime = *job.Status.CompletionTime
					crRep.Status.LastRestoreSpec = lastBakSpec
					if !reflect.DeepEqual(crRep.Status, instance.Status) {
						if err := r.Status().Update(ctx, crRep); err != nil {
							logger.Error(err, "Status update FAILED(phase: finalUpdate)")
							return ctrl.Result{}, err
						}
						return ctrl.Result{}, nil
					}
				}
			}
		case batchv1.JobFailed:
			{
				// JobIsFialed
				lastTransTime := job.Status.Conditions[len(job.Status.Conditions)-1].LastTransitionTime
				if crRep.Status.LastFailedTime.Before(&lastTransTime) {
					logger.Info("Have new job FAILED", "jobName", job.Name)
					meta.SetStatusCondition(&crRep.Status.Conditions, metav1.Condition{
						Type:               "Failed",
						Status:             metav1.ConditionTrue,
						Reason:             "JobFailed",
						Message:            "Job is Failed",
						LastTransitionTime: metav1.NewTime(time.Now()),
					})
					crRep.Status.LastFailedTime = lastTransTime
				}
				if !reflect.DeepEqual(crRep.Status, instance.Status) {
					if err := r.Status().Update(ctx, crRep); err != nil {
						logger.Error(err, "Status update FAILED(phase: finalUpdate)")
						return ctrl.Result{}, err
					}
					return ctrl.Result{}, nil
				}
			}
		}
	}
	return ctrl.Result{}, nil
}

func (r *MySQLRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set index
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, jobOwnerKey, func(rawObj client.Object) []string {
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != apiGvr || owner.Kind != "MySQLRestore" {
			return nil
		}
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&dbrestorev1alpha1.MySQLRestore{}).
		Owns(&batchv1.Job{}).
		Named("mysqlrestore").
		Complete(r)
}
