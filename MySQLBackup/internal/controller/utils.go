package controller

import (
	"context"
	dbbackupv1alpha1 "local/MySQLBackup/api/v1alpha1"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *MySQLBackupReconciler) BuildJobSruct(instance *dbbackupv1alpha1.MySQLBackup, bakPath string) (*batchv1.Job, error) {
	jobTemp := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: instance.Name + "job" + "-",
			Namespace:    instance.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To(int32(2)),
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
									SecretName: instance.Spec.DBAuth,
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:            "mysqldumper",
							Image:           "swr.cn-north-4.myhuaweicloud.com/shanwen_img/sqldumper:v1",
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
							Image:           "swr.cn-north-4.myhuaweicloud.com/shanwen_img/detector:v1",
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
		for i, _ := range jobTemp.Spec.Template.Spec.Containers {
			jobTemp.Spec.Template.Spec.Containers[i].Env = append(jobTemp.Spec.Template.Spec.Containers[i].Env, env...)
		}
		for i, _ := range jobTemp.Spec.Template.Spec.InitContainers {
			jobTemp.Spec.Template.Spec.InitContainers[i].Env = append(jobTemp.Spec.Template.Spec.InitContainers[i].Env, env...)
		}
	}

	addVolume := func(volumes []corev1.Volume) {
		jobTemp.Spec.Template.Spec.Volumes = append(jobTemp.Spec.Template.Spec.Volumes, volumes...)
	}

	addVolumeMount := func(volumesMnt []corev1.VolumeMount) {
		for i, _ := range jobTemp.Spec.Template.Spec.Containers {
			jobTemp.Spec.Template.Spec.Containers[i].VolumeMounts = append(jobTemp.Spec.Template.Spec.Containers[i].VolumeMounts, volumesMnt...)
		}
		for i, _ := range jobTemp.Spec.Template.Spec.InitContainers {
			jobTemp.Spec.Template.Spec.InitContainers[i].VolumeMounts = append(jobTemp.Spec.Template.Spec.InitContainers[i].VolumeMounts, volumesMnt...)
		}
	}

	// injection essential data to access mysql
	mysqlEnv := []corev1.EnvVar{
		{
			Name:  "rHost",
			Value: instance.Spec.Host,
		},
		{
			Name:  "rPort",
			Value: instance.Spec.Port,
		},
		{
			Name:  "rDBName",
			Value: instance.Spec.DBName,
		},
		{
			Name:  "BakMode",
			Value: string(instance.Spec.BakMode),
		},
	}
	addEnv(mysqlEnv)

	// 为S3Mode添加必要的参数
	if instance.Spec.BakMode == dbbackupv1alpha1.StorageModeS3 {
		// 为S3Mode添加Env
		s3Env := []corev1.EnvVar{
			corev1.EnvVar{
				Name:  "S3Endpoint",
				Value: instance.Spec.S3Bak.Endpoint,
			},
			corev1.EnvVar{
				Name:  "S3Bucket",
				Value: instance.Spec.S3Bak.Bucket,
			},
			corev1.EnvVar{
				Name:  "bakPath",
				Value: bakPath,
			},
		}
		addEnv(s3Env)

		// 定义S3AuthVolume
		s3Auth := []corev1.Volume{
			{
				Name: "s3auth",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: instance.Spec.S3Bak.S3Auth,
					},
				},
			},
		}
		addVolume(s3Auth)

		// 定义S3AuthVolumeMount
		s3AuthMount := []corev1.VolumeMount{
			{
				Name:      "s3auth",
				MountPath: "/s3Auth",
				ReadOnly:  true,
			},
		}
		addVolumeMount(s3AuthMount)
	}

	// 设置属主
	if error := ctrl.SetControllerReference(instance, jobTemp, r.Scheme); error != nil {
		return nil, error
	}
	return jobTemp, nil
}

func (r *MySQLBackupReconciler) CheckSecret(instance *dbbackupv1alpha1.MySQLBackup, ctx context.Context, SecretName string) bool {
	err := r.Get(ctx, types.NamespacedName{Name: SecretName, Namespace: instance.Namespace}, &corev1.Secret{})
	if err != nil || apierrors.IsNotFound(err) {
		return false
	}
	return true
}

func (r *MySQLBackupReconciler) FetchS3File(instance *dbbackupv1alpha1.MySQLBackup, filePath string) (err error) {
	secretName := instance.Spec.S3Bak.S3Auth
	secret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: instance.Namespace}, secret); err != nil {
		return err
	}

	ac := secret.Data["ac"]
	sc := secret.Data["sc"]
	endpoint := strings.Split(instance.Spec.S3Bak.Endpoint, "http://")[1]
	buckets := instance.Spec.S3Bak.Bucket

	// Initialize minio client object.
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(string(ac), string(sc), ""),
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	objectCh := minioClient.ListObjects(ctx, buckets, minio.ListObjectsOptions{
		Prefix:    filePath,
		Recursive: true,
	})
	for object := range objectCh {
		if object.Err != nil {
			return err
		}
	}
	return nil
}

func (r *MySQLBackupReconciler) DeletesS3File(instance *dbbackupv1alpha1.MySQLBackup) (err error) {
	secretName := instance.Spec.S3Bak.S3Auth
	secret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: instance.Namespace}, secret); err != nil {
		return err
	}

	ac := secret.Data["ac"]
	sc := secret.Data["sc"]
	endpoint := strings.Split(instance.Spec.S3Bak.Endpoint, "http://")[1]
	buckets := instance.Spec.S3Bak.Bucket

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(string(ac), string(sc), ""),
	})
	if err != nil {
		return err
	}

	objectsCh := make(chan minio.ObjectInfo)
	go func() {
		defer close(objectsCh)

		for _, versionEntry := range instance.Status.AvailableVersion {
			objectToDelete := minio.ObjectInfo{
				Key: versionEntry.Path,
			}
			objectsCh <- objectToDelete
		}
	}()

	opts := minio.RemoveObjectsOptions{
		GovernanceBypass: true,
	}
	for rErr := range minioClient.RemoveObjects(context.Background(), buckets, objectsCh, opts) {
		return &rErr
	}
	return nil
}
