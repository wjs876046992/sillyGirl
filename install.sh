n="sillyplus"
s="/usr/local/$n"
a=arm64
if [[ $(uname -a | grep "x86_64") != "" ]]; then 
    a=amd64
fi ;
if [ ! -d $s ]; then 
    mkdir $s
fi ;
cd $s;
rm -rf $n;
v=`curl https://gitlab.com/cdle/$a/-/raw/main/compile_time.go --silent | tr -cd "[0-9]"`
d=""
if [ ${#v} == 13 ]; then
    d="https://gitlab.com/cdle/${a}/-/raw/main/sillyplus_linux_${a}_${v}"
else
    v=`curl https://raw.githubusercontent.com/cdle/binary/main/compile_time.go --silent | tr -cd "[0-9]"`
    if [ ${#v} == 13 ]; then
        d="https://raw.githubusercontent.com/cdle/binary/main/sillyplus_linux_${a}_${v}"
    else
        echo "Sorry，你网不好！"
        exit
    fi
fi
echo "检测到版本 $v"
echo "正在从 $d 下载..."
curl -o $n $d && chmod 777 $n
echo "傻妞已安装到 $s"
echo "请手动运行 $s/$n -t"