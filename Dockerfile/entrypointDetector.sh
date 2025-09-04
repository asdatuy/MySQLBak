#!/bin/sh
sqlTest(){
    user=$(cat /sqlCredential/* | grep user | awk -F " " '{print $2}')
    passwd=$(cat /sqlCredential/* | grep passwd | awk -F " " '{print $2}')

    if [[ -z $user || -z $passwd || -z $rHost || -z $rPort || -z $rDBName ]]
    then
        echo "Insufficient information provided" >&2
        exit 1
    fi
    echo "try to access db"

    mysql -u${user} -p${passwd} -h${rHost} -P${rPort} ${rDBName} --skip-column-names -e 'select 1' 2> /dev/null
    if [[ $? != 0 ]]
    then
        echo "Dont access db" >&2
        exit 2
    fi
    echo "DB Access Success"
}

s3Test(){
    ac=$(cat /s3Auth/* | grep ac | awk -F " " '{print $2}')
    sc=$(cat /s3Auth/* | grep sc | awk -F " " '{print $2}')

    # 验证连接
    mcli alias set myminio/ ${S3Endpoint} ${ac} ${sc}
    mcli admin info myminio/ 2> /dev/null
    if [[ $? != 0 ]]
    then
        echo "S3 connection failed" >&2
        exit 3
    fi
    echo "S3 Access Success"

    # 验证桶的存在
    mcli ls myminio/${S3Bucket} 2> /dev/null
    if [[ $? != 0 ]]
    then
        echo "S3 bucket not exist" >&2
        exit 4
    fi
    echo "S3 Bucket Exist"
}

sqlTest
case "${BakMode}" in
    "s3")
        echo "In s3 storage Mode"
        s3Test
    ;;
    "pv")
        echo "In pv storage Mode"
        # TODO: pv Create
    ;;
    *)
        echo "${BakMode} is empty or error("s3","pv",other), Now use emptyDir"
    ;;
esac

echo -e "Remoute check succeed continue!"
exit 0
