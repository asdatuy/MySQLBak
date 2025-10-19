package controller

import (
	"context"
	"fmt"
	dbbackupv1alpha1 "local/MySQLBackup/api/v1alpha1"
	"reflect"
	"sort"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/prometheus/client_golang/prometheus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	metrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

type MySQLBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

var (
	jobOwnerKey     = ".metadata.controller"
	apiGVStr        = dbbackupv1alpha1.GroupVersion.String()
	myFinalizerName = "dbbackup.local.com/finalizer"
	PbakTotalJob    = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "bakJobTotal",
			Help: "Number of total jobs",
		},
	)
	PsucceedJobs = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "succeedJobs",
			Help: "Number of succeed jobs",
		},
	)
	PfailedJobs = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "bakJobfiled",
			Help: "Number of failed jobs",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(PbakTotalJob, PsucceedJobs, PfailedJobs)
}

// +kubebuilder:rbac:groups=dbbackup.local.com,resources=mysqlbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbbackup.local.com,resources=mysqlbackups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbbackup.local.com,resources=mysqlbackups/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get;list;watch;create;update;patch;delete
func (r *MySQLBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	// 确认能连接上 Apiserver 并存在 Instance
	instance := &dbbackupv1alpha1.MySQLBackup{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("[Instance]No CRD instance avliable")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "[CRD]Not fetch CRD")
		return ctrl.Result{}, err
	}

	// Finalizer
	if instance.DeletionTimestamp.IsZero() && !controllerutil.ContainsFinalizer(instance, myFinalizerName) {
		if changed := controllerutil.AddFinalizer(instance, myFinalizerName); changed {
			if err := r.Update(ctx, instance); err != nil {
				logger.Error(err, "[Finalizer]Faild add finalizer tags")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, nil
	} else if !instance.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(instance, myFinalizerName) {
		if err := r.DeletesS3File(instance); err != nil {
			return ctrl.Result{}, err
		}
		if changed := controllerutil.RemoveFinalizer(instance, myFinalizerName); changed {
			if err := r.Update(ctx, instance); err != nil {
				logger.Error(err, "[Finalizer]Faild remove finalizer tags")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, nil
	}

	// 为没有status的CR添加状态
	if len(instance.Status.Conditions) == 0 {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               "unkonw",
			Status:             metav1.ConditionUnknown,
			Reason:             "Reconcing",
			Message:            "Start Reconcil",
			LastTransitionTime: metav1.NewTime(time.Now()),
		})
		instance.Status.LatestActiveTime = metav1.Time{Time: time.Unix(0, 0)}
		instance.Status.LatestSucceedTime = metav1.Time{Time: time.Unix(0, 0)}
		instance.Status.LatestFailedTime = metav1.Time{Time: time.Unix(0, 0)}
		instance.Status.AvailableVersion = []dbbackupv1alpha1.AvailableVersion{}
		instance.Status.LastBakStatus = "unknow"
		instance.Status.LastReason = "unknow"
		// 更新状态
		if err := r.Status().Update(ctx, instance); err != nil {
			logger.Error(err, "[InitStatus]Failed update status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// 获取由当前CR实例拥有的Job
	var JobList batchv1.JobList
	if err := r.List(ctx, &JobList, client.InNamespace(req.Namespace), client.MatchingFields{jobOwnerKey: req.Name}); err != nil {
		logger.Error(err, "Error to fetch job owner by CRD")
		return ctrl.Result{}, err
	}

	// 对JobList进行CreateTime的排序
	sort.Slice(JobList.Items, func(i int, j int) bool {
		return JobList.Items[i].CreationTimestamp.Before(&JobList.Items[j].CreationTimestamp)
	})

	// 获取Job最后的状态信息
	isJobFinished := func(job *batchv1.Job) (bool, batchv1.JobConditionType) {
		for _, v := range job.Status.Conditions {
			if (v.Type == batchv1.JobComplete || v.Type == batchv1.JobFailed) && v.Status == corev1.ConditionTrue {
				return true, v.Type
			}
		}
		return false, ""
	}

	crRep := instance.DeepCopy()
	// 手动触发与job为0时创建Job
	if len(JobList.Items) == 0 || crRep.Spec.ManualTrigger == true {
		AuthList := []string{crRep.Spec.DBAuth, crRep.Spec.S3Bak.S3Auth}
		for _, auth := range AuthList {
			dbAuthExist := r.CheckSecret(crRep, ctx, auth)
			if !dbAuthExist {
				logger.Info("[Auth]Not Found", "Secret", auth)
				if changed := meta.SetStatusCondition(&crRep.Status.Conditions, metav1.Condition{
					Type:               "error",
					Status:             metav1.ConditionFalse,
					Reason:             "DBCredentailsNotFound",
					Message:            fmt.Sprintf("Secret %v Not Found", auth),
					LastTransitionTime: metav1.NewTime(time.Now()),
				}); changed {
					if err := r.Status().Update(ctx, crRep); err != nil {
						logger.Error(err, "[Status]Failed AuthCheck")
						return ctrl.Result{}, err
					}
					return ctrl.Result{}, nil
				}
			}
		}

		// 构建Job清单
		timeStep := metav1.NewTime(time.Now()).Format("2006-01-02T15:04:05Z")
		bakPath := crRep.Namespace + "/" + string(crRep.UID) + "/" + crRep.Spec.Host + ":" + crRep.Spec.Port + "/" + crRep.Spec.DBName + "/" + timeStep + ".zst"
		jobCreate, err := r.BuildJobSruct(crRep, bakPath)
		if err != nil {
			logger.Error(err, "[JobTemp]Failed to create jobTemp")
			if changed := meta.SetStatusCondition(&crRep.Status.Conditions, metav1.Condition{
				Type:               "error",
				Status:             metav1.ConditionFalse,
				Reason:             "JobTempCreateError",
				Message:            "JobTemp Create Error",
				LastTransitionTime: metav1.NewTime(time.Now()),
			}); changed {
				if err := r.Status().Update(ctx, crRep); err != nil {
					logger.Error(err, "[Status]Failed TempCreate")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}
		}

		// 在集群中创建Job
		if err := r.Create(ctx, jobCreate); err != nil {
			if changed := meta.SetStatusCondition(&crRep.Status.Conditions, metav1.Condition{
				Type:               "error",
				Status:             metav1.ConditionFalse,
				Reason:             "JobCreateError",
				Message:            "JobCreateError",
				LastTransitionTime: metav1.NewTime(time.Now()),
			}); changed {
				if err := r.Status().Update(ctx, crRep); err != nil {
					logger.Error(err, "[Status]Failed JobCreate")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}
		}

		// Patch Status
		patch := client.MergeFrom(crRep.DeepCopy())
		crRep.Status.AvailableVersion = append(crRep.Status.AvailableVersion, dbbackupv1alpha1.AvailableVersion{
			Path:         bakPath,
			Server:       crRep.Spec.S3Bak.Endpoint,
			BackTimeStep: timeStep,
			Available:    "unknow",
		})
		if !reflect.DeepEqual(instance.Status, crRep.Status) {
			if err := r.Status().Patch(ctx, crRep, patch); err != nil {
				logger.Error(err, "[Status]Failed update Status")
				return ctrl.Result{}, err
			}
		}

		if crRep.Spec.ManualTrigger {
			retry.RetryOnConflict(retry.DefaultRetry, func() error {
				//latest := &dbbackupv1alpha1.MySQLBackup{}
				//if err := r.Get(ctx, req.NamespacedName, latest); err != nil {
				//	return err
				//}
				//latest.Spec.ManualTrigger = false
				crRep.Spec.ManualTrigger = false
				return r.Update(ctx, crRep)
			})
		}
		return ctrl.Result{}, nil
	}

	// sort Jobs and Set Status
	for _, v := range JobList.Items {
		_, jobFinStatus := isJobFinished(&v)
		switch jobFinStatus {
		case "":
			{
				if crRep.Status.LatestActiveTime.Before(&v.CreationTimestamp) {
					PbakTotalJob.Inc()
					logger.Info("[Prometheus]Active+1", "JobName", v.Name, "JobCreateTime", v.CreationTimestamp)

					patch := client.MergeFrom(crRep.DeepCopy())
					crRep.Status.LatestActiveTime = v.CreationTimestamp
					meta.SetStatusCondition(&crRep.Status.Conditions, metav1.Condition{
						Type:               "Shcdulered",
						Status:             metav1.ConditionTrue,
						Reason:             "jobSchedulered",
						Message:            "jobIsRunning",
						LastTransitionTime: metav1.NewTime(time.Now()),
					})
					crRep.Status.LastBakStatus = "running"
					crRep.Status.LastReason = "Job is running"

					if !reflect.DeepEqual(instance.Status, crRep.Status) {
						if err := r.Status().Patch(ctx, crRep, patch); err != nil {
							// if err := r.Status().Update(ctx, crRep); err != nil {
							logger.Error(err, "[Status]FailedStausUpdate JobActive")
							return ctrl.Result{}, err
						}
						//return ctrl.Result{}, nil
					}
				}
			}
		case batchv1.JobComplete:
			{
				if crRep.Status.LatestSucceedTime.Before(v.Status.CompletionTime) {
					PsucceedJobs.Inc()
					logger.Info("[Prometheus]Succeed+1", "JobName", v.Name, "JobCreateTime", v.CreationTimestamp)

					patch := client.MergeFrom(crRep.DeepCopy())
					crRep.Status.LatestSucceedTime = *v.Status.CompletionTime
					crRep.Status.LastBakStatus = "completed"
					crRep.Status.LastBakStatus = "Job is succeed"

					meta.SetStatusCondition(&crRep.Status.Conditions, metav1.Condition{
						Type:               "Available",
						Status:             metav1.ConditionTrue,
						Reason:             "JobCompleted",
						Message:            "All pods of job is completed",
						LastTransitionTime: metav1.NewTime(time.Now()),
					})

					for i, v := range crRep.Status.AvailableVersion {
						if v.Available == "unknow" {
							if err := r.FetchS3File(crRep, v.Path); err != nil {
								crRep.Status.AvailableVersion[i].Available = "false"
							} else {
								crRep.Status.AvailableVersion[i].Available = "true"
								crRep.Status.AvailableVersion[i].Path = crRep.Spec.S3Bak.Bucket + "/" + v.Path
							}
						}
					}

					if !reflect.DeepEqual(instance.Status, crRep.Status) {
						if err := r.Status().Patch(ctx, crRep, patch); err != nil {
							logger.Error(err, "[Status]FailedStausUpdate JobCompleted")
							return ctrl.Result{}, err
						}
					}
				}
			}
		case batchv1.JobFailed:
			{
				if crRep.Status.LatestFailedTime.Before(&v.Status.Conditions[len(v.Status.Conditions)-1].LastTransitionTime) {
					PfailedJobs.Inc()
					logger.Info("[Prometheus]Failed+1", "JobName", v.Name, "JobCreateTime", v.CreationTimestamp)

					patch := client.MergeFrom(crRep.DeepCopy())
					crRep.Status.LatestFailedTime = v.Status.Conditions[len(v.Status.Conditions)-1].LastTransitionTime
					meta.SetStatusCondition(&crRep.Status.Conditions, metav1.Condition{
						Type:               "Available",
						Status:             metav1.ConditionFalse,
						Reason:             "PodForJobsFailed",
						Message:            "Have pods of job status is failed",
						LastTransitionTime: metav1.NewTime(time.Now()),
					})
					crRep.Status.LastBakStatus = "failed"
					crRep.Status.LastReason = "Job is failed"

					if !reflect.DeepEqual(instance.Status, crRep.Status) {
						if err := r.Status().Patch(ctx, crRep, patch); err != nil {
							logger.Error(err, "[Status]FailedStausUpdate JobFailed")
							return ctrl.Result{}, err
						}
						//return ctrl.Result{}, nil
					}
				}
			}
		}
	}
	if !reflect.DeepEqual(instance.Status, crRep.Status) {
		if err := r.Status().Update(ctx, crRep); err != nil {
			logger.Error(err, "[Status] Final Stage")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	logger.Info("[End]Nothing to do")
	return ctrl.Result{}, nil
}

// if len(activeJob) > 0 {
// 	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
// 		Type:               "Shcdulered",
// 		Status:             metav1.ConditionTrue,
// 		Reason:             "jobSchedulered",
// 		Message:            "Job is running!!! ",
// 		LastTransitionTime: metav1.NewTime(time.Now()),
// 	})
// 	instance.Status.LastBakStatus = "running"
// 	instance.Status.LastReason = "Job is running"
// 	if err := r.Status().Update(ctx, instance); err != nil {
// 		logger.Error(err, "Failed update status")
// 		return ctrl.Result{}, err
// 	}
// 	logger.Info("Activce Status update", "status", instance.Status.LastBakStatus)
// } else if len(failedJob) > 0 {
// 	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
// 		Type:               "Available",
// 		Status:             metav1.ConditionFalse,
// 		Reason:             "PodForJobsFailed",
// 		Message:            "Have pods of job %v status is failed",
// 		LastTransitionTime: metav1.NewTime(time.Now()),
// 	})
// 	instance.Status.LastBakStatus = "failed"
// 	instance.Status.LastReason = "Job is failed"
// 	if err := r.Status().Update(ctx, instance); err != nil {
// 		logger.Error(err, "Failed update status")
// 		return ctrl.Result{}, err
// 	}
// 	logger.Info("Failed Status update", "status", instance.Status.LastBakStatus)
// } else if len(completedJob) > 0 {
// 	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
// 		Type:               "Available",
// 		Status:             metav1.ConditionTrue,
// 		Reason:             fmt.Sprintf("Completed:%dof%d", len(completedJob), len(JobList.Items)),
// 		Message:            "All pods of job is completed",
// 		LastTransitionTime: metav1.NewTime(time.Now()),
// 	})
// 	instance.Status.LastBakStatus = "Completed"
// 	instance.Status.LastReason = "Job is succeed"
// 	if err := r.Status().Update(ctx, instance); err != nil {
// 		logger.Error(err, "Failed update status")
// 		return ctrl.Result{}, err
// 	}
// 	logger.Info("Succeed Status update ", "status", instance.Status.LastBakStatus)
// }

func (r *MySQLBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	//创建索引器
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, jobOwnerKey, func(rawObj client.Object) []string {
		// 获取所有Job
		job := rawObj.(*batchv1.Job)
		// 获取Job对应的主人
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}

		// 保证Job的主人是该CR实例
		// $> kubectl api-resources | grep mysql
		// mysqlbackups		dbbackup.local.com/v1alpha1         true         MySQLBackup
		if owner.APIVersion != apiGVStr || owner.Kind != "MySQLBackup" {
			return nil
		}
		return []string{owner.Name}
	}); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbbackupv1alpha1.MySQLBackup{}).
		Owns(&batchv1.Job{}). // Changed Logic 默认观察了所有的Job状态 导致每次创建Job至少是3次Resoncile触发
		Named("mysqlbackup").
		Complete(r)
}
