s3Storage(){
    user=$(cat /sqlCredential/* | grep user | awk -F " " '{print $2}')
    passwd=$(cat /sqlCredential/* | grep passwd | awk -F " " '{print $2}')

    ac=$(cat /s3Auth/* | grep ac | awk -F " " '{print $2}')
    sc=$(cat /s3Auth/* | grep sc | awk -F " " '{print $2}')

    mcli alias set myminio/ ${S3Endpoint} ${ac} ${sc}
    mysqldump -u${user} -p${passwd} -h${rHost} -P${rPort} ${rDBName} | zstd -c | mcli pipe myminio/${S3Bucket}/${rDBName}.zst
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
    "pv")
        echo "In pv storage Mode"
        # TODO: pv Create
    ;;
    *)
        echo "${BakMode} is empty or error("s3","pv",other), Now use emptyDir"
    ;;
esac
