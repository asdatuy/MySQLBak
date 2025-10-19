package controller

import (
	"context"
	dbrestorev1alpha1 "local/MySQLRestore/api/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
)

func IsJobFin(job *batchv1.Job) (bool, batchv1.JobConditionType) {
	for _, v := range job.Status.Conditions {
		if v.Type == batchv1.JobComplete || v.Type == batchv1.JobFailed && v.Status == corev1.ConditionTrue {
			return true, v.Type
		}
	}
	return false, ""
}

func (r *MySQLRestoreReconciler) BuildJobTmp(spec *dbrestorev1alpha1.MySQLRestore) (*batchv1.Job, error) {
	jobTmp := &batchv1.Job{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: spec.Name + "-job-",
			Namespace:    spec.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To(int32(2)),
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"dbbackupv1alpha1/MySQLBackup": "backjob"},
			},
			ManualSelector: ptr.To(true),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"dbbackupv1alpha1/MySQLBackup": "backjob"},
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "mysqlauth",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: spec.Spec.DbSpec.DbAuth,
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:            "mysqldumper",
							Image:           "swr.cn-north-4.myhuaweicloud.com/shanwen_img/importer:v1",
							ImagePullPolicy: corev1.PullIfNotPresent,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "mysqlauth",
									ReadOnly:  true,
									MountPath: "/sqlCredential",
								},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:            "detector",
							Image:           "swr.cn-north-4.myhuaweicloud.com/shanwen_img/detector:restorev1",
							ImagePullPolicy: corev1.PullIfNotPresent,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "mysqlauth",
									ReadOnly:  true,
									MountPath: "/sqlCredential",
								},
							},
						},
					},
				},
			},
		},
	}

	addEnv := func(env []corev1.EnvVar) {
		containers := jobTmp.Spec.Template.Spec.Containers
		initContainers := jobTmp.Spec.Template.Spec.InitContainers
		for i := range containers {
			jobTmp.Spec.Template.Spec.Containers[i].Env = append(jobTmp.Spec.Template.Spec.Containers[i].Env, env...)
		}
		for i := range initContainers {
			jobTmp.Spec.Template.Spec.InitContainers[i].Env = append(jobTmp.Spec.Template.Spec.InitContainers[i].Env, env...)
		}
	}

	addVolume := func(volumes []corev1.Volume) {
		jobTmp.Spec.Template.Spec.Volumes = append(jobTmp.Spec.Template.Spec.Volumes, volumes...)
	}

	addMount := func(volumeMount []corev1.VolumeMount) {
		containers := jobTmp.Spec.Template.Spec.Containers
		initContainers := jobTmp.Spec.Template.Spec.InitContainers
		for i := range containers {
			jobTmp.Spec.Template.Spec.Containers[i].VolumeMounts = append(jobTmp.Spec.Template.Spec.Containers[i].VolumeMounts, volumeMount...)
		}
		for i := range initContainers {
			jobTmp.Spec.Template.Spec.InitContainers[i].VolumeMounts = append(jobTmp.Spec.Template.Spec.InitContainers[i].VolumeMounts, volumeMount...)
		}
	}

	// injection essential variables
	sqlEnv := []corev1.EnvVar{
		corev1.EnvVar{
			Name:  "rHost",
			Value: spec.Spec.DbSpec.DbHost,
		},
		{
			Name:  "rPort",
			Value: spec.Spec.DbSpec.DbPort,
		},
		{
			Name:  "rDBName",
			Value: spec.Spec.DbSpec.DbName,
		},
	}
	addEnv(sqlEnv)

	// S3Spec
	s3Env := []corev1.EnvVar{
		corev1.EnvVar{
			Name:  "S3Path",
			Value: spec.Spec.S3Spec.Path,
		},
		{
			Name:  "S3Endpoint",
			Value: spec.Spec.S3Spec.Endpoint,
		},
	}
	addEnv(s3Env)

	// S3Secrets Volume and mount
	s3Secret := []corev1.Volume{
		{
			Name: "s3credentails",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: spec.Spec.S3Spec.S3Auth,
				},
			},
		},
	}
	s3Mount := []corev1.VolumeMount{
		{
			Name:      "s3credentails",
			ReadOnly:  true,
			MountPath: "/s3Auth",
		},
	}
	addVolume(s3Secret)
	addMount(s3Mount)

	// NewTargetMode set
	switch spec.Spec.ResotreModeSpec.ResotreMode {
	case dbrestorev1alpha1.NewTargetMode:
		{
			// TargetDestDBSpec
			targetSqlEnv := []corev1.EnvVar{
				corev1.EnvVar{
					Name:  "TargetMode",
					Value: string(spec.Spec.ResotreModeSpec.ResotreMode),
				},
				{
					Name:  "dHost",
					Value: spec.Spec.ResotreModeSpec.NewTargetModeSpec.DbSpec.DbHost,
				},
				{
					Name:  "dPort",
					Value: spec.Spec.ResotreModeSpec.NewTargetModeSpec.DbSpec.DbPort,
				},
				{
					Name:  "dDBName",
					Value: spec.Spec.ResotreModeSpec.NewTargetModeSpec.DbSpec.DbName,
				},
			}
			addEnv(targetSqlEnv)

			// TargetDestDBSecrest
			targetSqlSecret := []corev1.Volume{
				{
					Name: "targetsqldentails",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: spec.Spec.ResotreModeSpec.NewTargetModeSpec.DbSpec.DbAuth,
						},
					},
				},
			}
			targetSqlMount := []corev1.VolumeMount{
				{
					Name:      "targetsqldentails",
					ReadOnly:  true,
					MountPath: "/destSqlAuth",
				},
			}
			addVolume(targetSqlSecret)
			addMount(targetSqlMount)
		}
	case dbrestorev1alpha1.SourceTargetMode:
		{
			// SourceTargetMode Set
			tmpSqlEnv := []corev1.EnvVar{
				corev1.EnvVar{
					Name:  "TargetMode",
					Value: string(spec.Spec.ResotreModeSpec.ResotreMode),
				},
				{
					Name:  "BakDBName",
					Value: spec.Spec.ResotreModeSpec.SourceTargetModeSpec.BakDBName,
				},
				{
					Name:  "CrInstanceName",
					Value: spec.Name,
				},
			}
			addEnv(tmpSqlEnv)
		}
	}

	// if spec.Spec.ResotreModeSpec.ResotreMode == dbrestorev1alpha1.NewTargetMode {
	// }
	// if spec.Spec.ResotreModeSpec.ResotreMode == dbrestorev1alpha1.SourceTargetMode {
	// }

	// Set controller to index
	if err := ctrl.SetControllerReference(spec, jobTmp, r.Scheme); err != nil {
		return nil, err
	}
	return jobTmp, nil
}

func (r *MySQLRestoreReconciler) checkSecrets(cr *dbrestorev1alpha1.MySQLRestore, ctx context.Context, secretName string) error {
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cr.Namespace}, &corev1.Secret{}); err != nil {
		return err
	}
	return nil
}
