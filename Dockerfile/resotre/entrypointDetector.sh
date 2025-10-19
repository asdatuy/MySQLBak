#!/bin/sh
set -eo pipefail

sourceSqlTest(){
    user=$(cat /sqlCredential/* | grep user | awk -F " " '{print $2}')
    passwd=$(cat /sqlCredential/* | grep passwd | awk -F " " '{print $2}')

    if [[ -z $user || -z $passwd || -z $rHost || -z $rPort || -z $rDBName ]]
    then
        echo "Insufficient information provided"
        exit 1
    fi
    echo "try to access db"

    mysql -u${user} -p${passwd} -h${rHost} -P${rPort} ${rDBName} -e 'select 1'
    if [[ $? != 0 ]]
    then
        echo "Dont access source db"
        exit 1
    fi
    echo "Source DB Access Success"
}

targetSqlTest(){
    user=$(cat /destSqlAuth/* | grep user | awk -F " " '{print $2}')
    passwd=$(cat /destSqlAuth/* | grep passwd | awk -F " " '{print $2}')

    if [[ -z $user || -z $passwd || -z $dHost || -z $dPort || -z $dDBName ]]
    then
        echo "Insufficient information provided"
        exit 1
    fi
    echo "try to access db"

    mysql -u${user} -p${passwd} -h${dHost} -P${dPort} ${dDBName} -e 'select 1'
    if [[ $? != 0 ]]
    then
        echo "Dont access target db"
        exit 1
    fi
    echo "Target DB Access Success"
}

s3Test(){
    ac=$(cat /s3Auth/ac)
    sc=$(cat /s3Auth/sc)
    if [[ -z $ac || -z $sc ]]
    then
        echo "Insufficient information provided"
        exit 1
    fi
    echo "try to access S3"

    # 验证连接
    mcli alias set myminio/ ${S3Endpoint} ${ac} ${sc}
    mcli admin info myminio/ 2> /dev/null
    if [[ $? != 0 ]]
    then
        echo "S3 connection failed"
        exit 1
    fi
    echo "S3 Access Success"

    # 验证桶的存在
    mcli ls myminio/${S3Path} 2> /dev/null
    if [[ $? != 0 ]]
    then
        echo "S3 bucket not exist"
        exit 1
    fi
    echo "S3 Bucket Exist"

    # 还需要验证版本的存在
    echo -e "Version Exist\nS3Check Succeed"
}

sourceSqlTest
if [[ ${TargetMode} == "NewTargetMode" ]]; then
    targetSqlTest
fi
s3Test

echo -e "All Check succeed and continue!"
exit 0
