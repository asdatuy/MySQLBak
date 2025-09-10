package controller

import (
	"context"
	"fmt"
	dbbackupv1alpha1 "local/MySQLBackup/api/v1alpha1"
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	jobOwnerKey  = ".metadata.controller"
	apiGVStr     = dbbackupv1alpha1.GroupVersion.String()
	PbakTotalJob = prometheus.NewCounter(
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

	instance := &dbbackupv1alpha1.MySQLBackup{}
	err := r.Get(ctx, req.NamespacedName, instance)
	// 确认能连接上 Apiserver 并存在 Instance
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Error(err, "No CRD instance avliable")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Not fetch CRD")
		return ctrl.Result{}, err
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
		// 更新状态
		if err := r.Status().Update(ctx, instance); err != nil {
			logger.Error(err, "Failed update status")
			return ctrl.Result{}, err
		}
		// 更新状态后获取CR
		if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
			logger.Error(err, "Re-fetch failed")
			return ctrl.Result{}, err
		}
	}

	// 获取CR副本
	crStatusRep := instance.Status.DeepCopy()

	// 获取Job最后的状态信息
	isJobFinished := func(job *batchv1.Job) (bool, batchv1.JobConditionType) {
		for _, v := range job.Status.Conditions {
			if (v.Type == batchv1.JobComplete || v.Type == batchv1.JobFailed) && v.Status == corev1.ConditionTrue {
				return true, v.Type
			}
		}
		return false, ""
	}

	// 获取由当前CR实例拥有的Job
	var JobList batchv1.JobList
	if err := r.List(ctx, &JobList, client.InNamespace(req.Namespace), client.MatchingFields{jobOwnerKey: req.Name}); err != nil {
		logger.Error(err, "Error to fetch job owner by CRD")
		return ctrl.Result{}, err
	}

	// 手动触发与job为0时创建Job
	if len(JobList.Items) == 0 || instance.Spec.ManualTrigger == true {
		if instance.Spec.ManualTrigger {
			logger.Info("Manual Trigger", "ManualTrigger", instance.Spec.ManualTrigger)
			instance.Spec.ManualTrigger = false
			if err = r.Update(ctx, instance); err != nil {
				logger.Error(err, "Error to update manualTrigger", instance.Name)
				return ctrl.Result{}, err
			}
		}
		// 检查DBAuth是否存在
		dbAuthExist := r.CheckSecret(instance, ctx, instance.Spec.DBAuth)
		if !dbAuthExist {
			logger.Error(err, "Failed to get DBAuth", "Secret", instance.Spec.DBAuth)
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               "error",
				Status:             metav1.ConditionFalse,
				Reason:             "DBCredentailsNotFound",
				Message:            fmt.Sprintf("Secret %v Not Found", instance.Spec.DBAuth),
				LastTransitionTime: metav1.NewTime(time.Now()),
			})
			if err := r.Status().Update(ctx, instance); err != nil {
				logger.Error(err, "Failed update status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		logger.Info("Found DBAuth!!!", "Secret", instance.Spec.DBAuth)
		// 检查S3Auth是否存在
		s3AuthExist := r.CheckSecret(instance, ctx, instance.Spec.S3Bak.S3Auth)
		if !s3AuthExist {
			logger.Error(err, "Failed to get s3Auth", "Secret", instance.Spec.S3Bak.S3Auth)
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               "error",
				Status:             metav1.ConditionFalse,
				Reason:             "s3CredentailsNotFound",
				Message:            fmt.Sprintf("Secret %v Not Found", instance.Spec.S3Bak.S3Auth),
				LastTransitionTime: metav1.NewTime(time.Now()),
			})
			if err := r.Status().Update(ctx, instance); err != nil {
				logger.Error(err, "Failed update status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		logger.Info("Found S3Auth!!!", "Secret", instance.Spec.S3Bak.S3Auth)
		// 构建Job清单
		tempJobName := instance.Name + "-" + strconv.Itoa(len(JobList.Items)+1)
		jobCreate, err := r.BuildJobSruct(instance, tempJobName)
		if err != nil {
			logger.Error(err, "Failed to create jobTemp")
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               "error",
				Status:             metav1.ConditionFalse,
				Reason:             "JobTempCreateError",
				Message:            fmt.Sprintf("JobTemp %v Create Error", jobCreate.Name),
				LastTransitionTime: metav1.NewTime(time.Now()),
			})
			if err := r.Status().Update(ctx, instance); err != nil {
				logger.Error(err, "Failed update status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		// 在集群中创建Job
		logger.Info("Will create job", "JobName", jobCreate.Name, "namespace", jobCreate.Namespace)
		if err = r.Create(ctx, jobCreate); err != nil {
			logger.Error(err, "Failed to create job")
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               "error",
				Status:             metav1.ConditionFalse,
				Reason:             "JobCreateError",
				Message:            fmt.Sprintf("Job %v Create Error", jobCreate.Name),
				LastTransitionTime: metav1.NewTime(time.Now()),
			})
			if err := r.Status().Update(ctx, instance); err != nil {
				logger.Error(err, "Failed update status")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// 对JobList进行CreateTime的排序 required by prometheus count
	sort.Slice(JobList.Items, func(i int, j int) bool {
		return JobList.Items[i].CreationTimestamp.Before(&JobList.Items[j].CreationTimestamp)
	})

	// Water mark set and reset on nil
	// activeWaterMark := instance.Status.LatestCreateTime
	// succedWaterMark := instance.Status.LatestSucceedTime
	// failedWaterMark := instance.Status.LatestFailedTime
	activeWaterMark := crStatusRep.LatestCreateTime
	succedWaterMark := crStatusRep.LatestSucceedTime
	failedWaterMark := crStatusRep.LatestFailedTime
	if activeWaterMark == nil {
		activeWaterMark = &metav1.Time{}
	}
	if succedWaterMark == nil {
		succedWaterMark = &metav1.Time{}
	}
	if failedWaterMark == nil {
		failedWaterMark = &metav1.Time{}
	}

	var activeJob []*batchv1.Job
	var failedJob []*batchv1.Job
	var completedJob []*batchv1.Job
	// tmpActiveWaterMark := *activeWaterMark
	// tmpSuccedWaterMark := *succedWaterMark
	// tmpFailedWaterMark := *failedWaterMark
	for _, v := range JobList.Items {
		_, jobFinStatus := isJobFinished(&v)
		switch jobFinStatus {
		case "":
			{
				//logger.Info("Job Avtive", "JobName", v.Name, "CreateTime", v.CreationTimestamp)
				if activeWaterMark.Before(&v.CreationTimestamp) {
					activeJob = append(activeJob, &v)
					crStatusRep.LatestCreateTime = &v.CreationTimestamp
					// tmpActiveWaterMark = v.CreationTimestamp
					PbakTotalJob.Inc()
					logger.Info("New Active Jobs and Prometheus+1", "JobName", v.Name, "JobCreateTime", v.CreationTimestamp)
					// logger.Info("activeWaterMark", "activeWaterMark", activeWaterMark)
					// 设置临时crStaus
					meta.SetStatusCondition(&crStatusRep.Conditions, metav1.Condition{
						Type:               "Shcdulered",
						Status:             metav1.ConditionTrue,
						Reason:             "jobSchedulered",
						Message:            "Job is running!!! ",
						LastTransitionTime: metav1.NewTime(time.Now()),
					})
					crStatusRep.LastBakStatus = "running"
					crStatusRep.LastReason = "Job is running"
				}
			}
		case batchv1.JobComplete:
			{
				//logger.Info("Job Completed", "JobName", v.Name, "Time", v.Status.CompletionTime)
				if succedWaterMark.Before(v.Status.CompletionTime) {
					completedJob = append(completedJob, &v)
					PsucceedJobs.Inc()
					crStatusRep.LatestSucceedTime = v.Status.CompletionTime
					// tmpSuccedWaterMark = *v.Status.CompletionTime
					logger.Info("New Succeed Jobs and Prometheus+1", "JobName", v.Name, "JobSucceedTime", v.Status.CompletionTime)
					// logger.Info("succeedWaterMark", "succeedWaterMark", succedWaterMark)
					// 设置临时crStaus
					meta.SetStatusCondition(&crStatusRep.Conditions, metav1.Condition{
						Type:               "Available",
						Status:             metav1.ConditionTrue,
						Reason:             fmt.Sprintf("Completed:%dof%d", len(completedJob), len(JobList.Items)),
						Message:            "All pods of job is completed",
						LastTransitionTime: metav1.NewTime(time.Now()),
					})
					crStatusRep.LastBakStatus = "completed"
					crStatusRep.LastReason = "Job is succeed"
				}
			}
		case batchv1.JobFailed:
			{
				//logger.Info("Job Failed", "JobName", v.Name)
				if failedWaterMark.Before(&v.Status.Conditions[len(v.Status.Conditions)-1].LastTransitionTime) {
					failedJob = append(failedJob, &v)
					PfailedJobs.Inc()
					crStatusRep.LatestFailedTime = &v.Status.Conditions[len(v.Status.Conditions)-1].LastTransitionTime
					// tmpFailedWaterMark = v.Status.Conditions[len(v.Status.Conditions)-1].LastTransitionTime
					logger.Info("New Failed Jobs and Prometheus+1", "jobName", v.Name, "JobFailedTime", v.Status.Conditions[len(v.Status.Conditions)-1].LastTransitionTime)
					//logger.Info("failedMasterMark", "time", failedWaterMark)
					// 设置临时cr
					meta.SetStatusCondition(&crStatusRep.Conditions, metav1.Condition{
						Type:               "Available",
						Status:             metav1.ConditionFalse,
						Reason:             "PodForJobsFailed",
						Message:            "Have pods of job %v status is failed",
						LastTransitionTime: metav1.NewTime(time.Now()),
					})
					crStatusRep.LastBakStatus = "failed"
					crStatusRep.LastReason = "Job is failed"
				}
			}
		}
	}

	if !reflect.DeepEqual(crStatusRep, instance.Status) {
		instance.Status = *crStatusRep.DeepCopy()
		if err = r.Status().Update(ctx, instance); err != nil {
			logger.Error(err, "Update status failed phase: final update")
			return ctrl.Result{}, err
		}
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
	return ctrl.Result{}, nil
}

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
		Owns(&batchv1.Job{}).
		Named("mysqlbackup").
		Complete(r)
}
