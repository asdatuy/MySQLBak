podman build -f DOCKERFILEDetector --tag swr.cn-north-4.myhuaweicloud.com/shanwen_img/detector:v1
podman push swr.cn-north-4.myhuaweicloud.com/shanwen_img/detector:v1
podman build -f DOCKERFILEDumper --tag swr.cn-north-4.myhuaweicloud.com/shanwen_img/sqldumper:v1
podman push swr.cn-north-4.myhuaweicloud.com/shanwen_img/sqldumper:v1
