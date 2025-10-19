podman build -f DOCKERFILEDetector --tag swr.cn-north-4.myhuaweicloud.com/shanwen_img/detector:restorev1
podman push swr.cn-north-4.myhuaweicloud.com/shanwen_img/detector:restorev1 
podman build -f DOCKERFILEImporter --tag swr.cn-north-4.myhuaweicloud.com/shanwen_img/importer:v1
podman push swr.cn-north-4.myhuaweicloud.com/shanwen_img/importer:v1
