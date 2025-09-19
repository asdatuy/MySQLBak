#!/bin/env sh
set -eo pipefail
NewTargetMode(){
    user=$(cat /sqlCredential/* | grep user | awk -F " " '{print $2}')
    passwd=$(cat /sqlCredential/* | grep passwd | awk -F " " '{print $2}')
    # destSqlAuth
    duser=$(cat /destSqlAuth/* | grep user | awk -F " " '{print $2}')
    dpasswd=$(cat /destSqlAuth/* | grep passwd | awk -F " " '{print $2}')

    ac=$(cat /s3Auth/* | grep ac | awk -F " " '{print $2}')
    sc=$(cat /s3Auth/* | grep sc | awk -F " " '{print $2}')

    mcli alias set myminio/ ${S3Endpoint} ${ac} ${sc}
    if [[ ${S3Version} == "latest" ]]; then
        mcli cat myminio/${S3Bucket}/${rDBName}.zst | zstdcat | mysql -u${user} -p${passwd} -P${dPort} -h${dHost} ${dDBName}
        if [[ $? != 0 ]]
        then
            echo "Restore failed"
            exit 1
        fi
        echo "Resotre Succed"
        exit 0
    fi

    mcli cat --version-id ${S3Version} myminio/${S3Bucket}/${rDBName}.zst | zstdcat | mysql -u${user} -p${passwd} -P${dPort} -h${dHost} ${dDBName}
    if [[ $? != 0 ]]
    then
        echo "Resotre failed"
        exit 1
    fi
    echo "Resotre Succed"
    exit 0
}

SourceTargetMode(){
    user=$(cat /sqlCredential/* | grep user | awk -F " " '{print $2}')
    passwd=$(cat /sqlCredential/* | grep passwd | awk -F " " '{print $2}')

    ac=$(cat /s3Auth/* | grep ac | awk -F " " '{print $2}')
    sc=$(cat /s3Auth/* | grep sc | awk -F " " '{print $2}')

    mcli alias set myminio/ ${S3Endpoint} ${ac} ${sc}
    ## Backup
    mysqldump --single-transaction -u${user} -p${passwd} -h${rHost} -P${rPort} ${rDBName} | zstd -c | mcli pipe myminio/dbrestorebackup/${CrInstanceName}_${rDBName}_${BakDBName}.zst
    if [[ $? != 0 ]]
    then
        echo "Backup Origin DB failed"
        exit 1
    fi
    echo "Origin DB is backup"
    ## Resotre
    if [[ ${S3Version} == "latest" ]]; then
        mcli cat myminio/${S3Bucket}/${rDBName}.zst | zstdcat | mysql -u${user} -p${passwd} -P${rPort} -h${rHost} ${rDBName}
        if [[ $? != 0 ]]
        then
            echo "Restore failed"
            exit 1
        fi
        echo "Resotre Succed"
        exit 0
    fi

    mcli cat --version-id ${S3Version} myminio/${S3Bucket}/${rDBName}.zst | zstdcat | mysql -u${user} -p${passwd} -P${rPort} -h${rHost} ${rDBName}
    if [[ $? != 0 ]]
    then
        echo "Restore failed"
        exit 1
    fi
    echo "Restore Succed"
    exit 0
}

case ${TargetMode} in
    "NewTargetMode")
        echo "Select newtarget mode"
        NewTargetMode
    ;;
    "SourceTargetMode")
        echo "Select source mode"
        SourceTargetMode
    ;;
    *)
        echo "${TargetMode} is empty or error("NewTargetMode","SourceTargetMode")"
        exit 1
    ;;
esac
