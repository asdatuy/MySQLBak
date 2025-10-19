s3Storage(){
    user=$(cat /sqlCredential/* | grep user | awk -F " " '{print $2}')
    passwd=$(cat /sqlCredential/* | grep passwd | awk -F " " '{print $2}')

    ac=$(cat /s3Auth/ac)
    sc=$(cat /s3Auth/sc)

    mcli alias set myminio/ ${S3Endpoint} ${ac} ${sc}
    mysqldump --single-transaction -u${user} -p${passwd} -h${rHost} -P${rPort} ${rDBName} | zstd -c | mcli pipe myminio/${S3Bucket}/${bakPath}
    if [[ $? != 0 ]]
    then
        echo "Backup db failed"
        exit 1
    fi
    echo "Backup Succed tar and stored to s3"
    exit 0
}

case "${BakMode}" in
    "s3")
        echo "In s3 storage Mode"
        s3Storage
    ;;
    *)
        echo "${BakMode} is empty or error("s3","pv",other), Now use emptyDir"
    ;;
esac
