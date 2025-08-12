package controller

import (
	"context"
	"fmt"
	dbbackupv1alpha1 "local/MySQLBackup/api/v1alpha1"
	"strconv"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type MySQLBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

var (
	jobOwnerKey = ".metadata.controller"
	apiGVStr    = dbbackupv1alpha1.GroupVersion.String()
)

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

	var activeJob []*batchv1.Job
	var failedJob []*batchv1.Job
	var completedJob []*batchv1.Job
	for _, v := range JobList.Items {
		_, jobFinStatus := isJobFinished(&v)
		switch jobFinStatus {
		case "":
			{
				logger.Info("Job Avtive", "JobName", v.Name)
				activeJob = append(activeJob, &v)
			}
		case batchv1.JobComplete:
			{
				logger.Info("Job Completed", "JobName", v.Name)
				completedJob = append(completedJob, &v)
			}
		case batchv1.JobFailed:
			{
				logger.Info("Job Failed", "JobName", v.Name)
				failedJob = append(failedJob, &v)
			}
		}
	}

	if len(activeJob) > 0 {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               "Progressing",
			Status:             metav1.ConditionTrue,
			Reason:             "JobRunning",
			Message:            "Job is running!!! ",
			LastTransitionTime: metav1.NewTime(time.Now()),
		})
	} else if len(failedJob) > 0 {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionFalse,
			Reason:             "PodForJobsFailed",
			Message:            "Have pods of job %v status is failed",
			LastTransitionTime: metav1.NewTime(time.Now()),
		})
	} else if len(completedJob) > 0 {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionTrue,
			Reason:             fmt.Sprintf("Completed:%dof%d", len(completedJob), len(JobList.Items)),
			Message:            "All pods of job is completed",
			LastTransitionTime: metav1.NewTime(time.Now()),
		})
	}

	if len(JobList.Items) == 0 || instance.Spec.ManualTrigger == true {
		if instance.Spec.ManualTrigger {
			instance.Spec.ManualTrigger = false
			if err = r.Update(ctx, instance); err != nil {
				logger.Error(err, "Error to update manualTrigger", instance.Name)
				return ctrl.Result{}, err
			}
		}

		// 查看集群中是否存在需要的资源(svc,secret)
		// svcFound := &corev1.Service{}
		// err := r.Get(ctx, types.NamespacedName{Name: instance.Spec.Host, Namespace: instance.Namespace}, svcFound)
		// if err != nil || apierrors.IsNotFound(err) {
		// 	logger.Error(err, "Failed to get svc", "Service", instance.Spec.Host)
		// 	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		// 		Type:               "error",
		// 		Status:             metav1.ConditionFalse,
		// 		Reason:             "HostNotFound",
		// 		Message:            fmt.Sprintf("Not found svc %v in this NS", instance.Spec.Host),
		// 		LastTransitionTime: metav1.NewTime(time.Now()),
		// 	})
		// 	if err := r.Status().Update(ctx, instance); err != nil {
		// 		logger.Error(err, "Failed update status")
		// 		return ctrl.Result{}, err
		// 	}
		// 	return ctrl.Result{}, err
		// }
		// logger.Info("Failed to get svc!!!", "Host", instance.Spec.Host)

		// 获取Credentails
		sctFound := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: instance.Spec.DBAuth, Namespace: instance.Namespace}, sctFound)
		if err != nil || apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to get Secret", "Secret", instance.Spec.DBAuth)
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               "error",
				Status:             metav1.ConditionFalse,
				Reason:             "CredentailsNotFound",
				Message:            fmt.Sprintf("Secret %v Not Found", instance.Spec.DBAuth),
				LastTransitionTime: metav1.NewTime(time.Now()),
			})
			if err := r.Status().Update(ctx, instance); err != nil {
				logger.Error(err, "Failed update status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		logger.Info("Found Secret!!!", "Secret", instance.Spec.DBAuth)

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
			return ctrl.Result{}, err
		}

		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               "Progressing",
			Status:             metav1.ConditionTrue,
			Reason:             "Jobcreated",
			Message:            "Job Created",
			LastTransitionTime: metav1.NewTime(time.Now()),
		})
	}

	// final update status
	if err := r.Status().Update(ctx, instance); err != nil {
		logger.Error(err, "Failed update status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *MySQLBackupReconciler) BuildJobSruct(job *dbbackupv1alpha1.MySQLBackup, jobName string) (*batchv1.Job, error) {
	jobTemp := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: job.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To(int32(6)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"dbbackupv1alpha1/MySQLBackup": "backjob"},
			},
			ManualSelector: ptr.To(true),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"dbbackupv1alpha1/MySQLBackup": "backjob"},
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "credentails",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: job.Spec.DBAuth,
								},
							},
						},
						{
							Name: "mysqldesc",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									Medium: corev1.StorageMediumDefault,
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:            "mysqldumper",
							Image:           "swr.cn-north-4.myhuaweicloud.com/shanwen_img/sqldump:v2",
							ImagePullPolicy: corev1.PullIfNotPresent,
							VolumeMounts: []corev1.VolumeMount{
								corev1.VolumeMount{
									Name:      "credentails",
									ReadOnly:  true,
									MountPath: "/sqlCredential",
								},
								corev1.VolumeMount{
									Name:      "mysqldesc",
									MountPath: "/mysqldesc",
								},
							},
						},
					},
					InitContainers: []corev1.Container{
						corev1.Container{
							Name:            "detector",
							Image:           "swr.cn-north-4.myhuaweicloud.com/shanwen_img/detector:v1",
							ImagePullPolicy: corev1.PullIfNotPresent,
							VolumeMounts: []corev1.VolumeMount{
								corev1.VolumeMount{
									Name:      "mysqldesc",
									MountPath: "/mysqldesc",
								},
							},
							Env: []corev1.EnvVar{
								corev1.EnvVar{
									Name:  "rHost",
									Value: job.Spec.Host,
								},
								corev1.EnvVar{
									Name:  "rPort",
									Value: job.Spec.Port,
								},
								corev1.EnvVar{
									Name:  "rDBName",
									Value: job.Spec.DBName,
								},
							},
						},
					},
				},
			},
		},
	}

	if error := ctrl.SetControllerReference(job, jobTemp, r.Scheme); error != nil {
		return nil, error
	}
	return jobTemp, nil
}

func (r *MySQLBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// 创建索引器
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, jobOwnerKey, func(rawObj client.Object) []string {
		// 查询Job对应的主人
		job := rawObj.(*batchv1.Job)
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
