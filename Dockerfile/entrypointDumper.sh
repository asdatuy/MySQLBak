user=$(cat * | grep user | awk -F " " '{print $2}')
passwd=$(cat * | grep passwd | awk -F " " '{print $2}')

host=$(cat /mysqldesc/file | grep host | awk -F " " '{print $2}')
port=$(cat /mysqldesc/file | grep port | awk -F " " '{print $2}')
db=$(cat /mysqldesc/file | grep db | awk -F " " '{print $2}')

if [[ -z $user || -z $passwd || -z $host || -z $port || -z $db ]]
then
    echo "No file providers data to access db"
    exit 1
fi
echo "Use file providers data to access db"
echo "Try to access db"

mysqlVersion=$(mysql -u${user} -p${passwd} -h${host} -P${port} ${db} --skip-column-names -e 'select version()') 2> /dev/nu
if [[ $? != 0 ]]
then
    echo "Dont access db"
    exit 2
fi

echo "Access DBVersion ${mysqlVersion} succeed , Start to backup ${db}"
mysqldump -u${user} -p${passwd} -h${host} -P${port} ${db} > /bakDir/${db}.sql
if [[ $? != 0 ]]
then
    echo "Backup db failed"
    exit 3
fi

echo "Backup Succed scripts will exit"
exit 0
