package controller

import (
	"local/MySQLBackup/api/v1alpha1"
	dbbackupv1alpha1 "local/MySQLBackup/api/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
)

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
							Name: "mysqlauth",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: job.Spec.DBAuth,
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:            "mysqldumper",
							Image:           "swr.cn-north-4.myhuaweicloud.com/shanwen_img/sqldump:v3",
							ImagePullPolicy: corev1.PullIfNotPresent,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "mysqlauth",
									ReadOnly:  true,
									MountPath: "/sqlCredential",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "init",
									Value: "done",
								},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:            "detector",
							Image:           "swr.cn-north-4.myhuaweicloud.com/shanwen_img/detector:v2",
							ImagePullPolicy: corev1.PullIfNotPresent,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "mysqlauth",
									ReadOnly:  true,
									MountPath: "/sqlCredential",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "init",
									Value: "done",
								},
							},
						},
					},
				},
			},
		},
	}

	// injection essential data to access mysql
	mysqlEnv := []corev1.EnvVar{
		{
			Name:  "rHost",
			Value: job.Spec.Host,
		},
		{
			Name:  "rPort",
			Value: job.Spec.Port,
		},
		{
			Name:  "rDBName",
			Value: job.Spec.DBName,
		},
		{
			Name:  "BakMode",
			Value: string(job.Spec.BakMode),
		},
	}

	for i := range jobTemp.Spec.Template.Spec.Containers {
		jobTemp.Spec.Template.Spec.Containers[i].Env = append(jobTemp.Spec.Template.Spec.Containers[i].Env, mysqlEnv...)
	}
	for i := range jobTemp.Spec.Template.Spec.InitContainers {
		jobTemp.Spec.Template.Spec.InitContainers[i].Env = append(jobTemp.Spec.Template.Spec.InitContainers[i].Env, mysqlEnv...)
	}

	// 为S3Mode添加必要的参数
	if job.Spec.BakMode == v1alpha1.StorageModeS3 {
		// 为S3Mode添加Env
		s3Env := []corev1.EnvVar{
			corev1.EnvVar{
				Name:  "S3Endpoint",
				Value: job.Spec.S3Bak.Endpoint,
			},
			corev1.EnvVar{
				Name:  "S3Bucket",
				Value: job.Spec.S3Bak.Bucket,
			},
		}
		for i := range jobTemp.Spec.Template.Spec.Containers {
			jobTemp.Spec.Template.Spec.Containers[i].Env = append(jobTemp.Spec.Template.Spec.Containers[i].Env, s3Env...)
		}
		for i := range jobTemp.Spec.Template.Spec.InitContainers {
			jobTemp.Spec.Template.Spec.InitContainers[i].Env = append(jobTemp.Spec.Template.Spec.InitContainers[i].Env, s3Env...)
		}

		// 定义S3AuthVolume
		s3Auth := corev1.Volume{
			Name: "s3auth",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: job.Spec.S3Bak.S3Auth,
				},
			},
		}

		// 定义S3AuthVolumeMount
		s3AuthMount := corev1.VolumeMount{
			Name:      "s3auth",
			MountPath: "/s3Auth",
			ReadOnly:  true,
		}

		// 应用于JobTemp中
		jobTemp.Spec.Template.Spec.Volumes = append(jobTemp.Spec.Template.Spec.Volumes, s3Auth)

		for i := range jobTemp.Spec.Template.Spec.Containers {
			jobTemp.Spec.Template.Spec.Containers[i].VolumeMounts = append(jobTemp.Spec.Template.Spec.Containers[i].VolumeMounts, s3AuthMount)
		}
		for i := range jobTemp.Spec.Template.Spec.InitContainers {
			jobTemp.Spec.Template.Spec.InitContainers[i].VolumeMounts = append(jobTemp.Spec.Template.Spec.InitContainers[i].VolumeMounts, s3AuthMount)
		}
	}

	if error := ctrl.SetControllerReference(job, jobTemp, r.Scheme); error != nil {
		return nil, error
	}
	return jobTemp, nil
}
