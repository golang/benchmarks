#!/bin/bash -x

# git fetch "https://go.googlesource.com/go" refs/changes/61/196661/3 && git checkout FETCH_HEAD

if [ $# -lt 1 ] ; then
  echo cmpcl.sh "refs/changes/<nn>/<cl>/<patch>" "bent-options"
  exit 1
fi

if [ ${1:0:1} = "-" ] ; then
  echo "First parameter should be a git tag or branch"
  exit 1
fi

cl="$1"
shift

ROOT=`pwd`
export ROOT cl

# perflock is not always available
PERFLOCK=`which perflock`

# N is number of benchmarks, B is number of builds
# Can override these with -N= and -a= on command line.
N=15
B=1

cd "${ROOT}"

if [ -e go-old ] ; then
	rm -rf go-old
fi

git clone https://go.googlesource.com/go go-old
if [ $? != 0 ] ; then
	echo git clone https://go.googlesource.com/go go-old FAILED
	exit 1
fi
cd go-old/src
git fetch "https://go.googlesource.com/go" "${cl}"
if [ $? != 0 ] ; then
	echo git fetch "https://go.googlesource.com/go" "${cl}"  failed
	exit 1
fi
git checkout FETCH_HEAD^1
if [ $? != 0 ] ; then
	echo git checkout FETCH_HEAD^1 failed
	exit 1
fi
./make.bash
if [ $? != 0 ] ; then
	echo BASE make.bash FAILED
	exit 1
fi
oldtag=`git log -n 1 --format='%h'`
export oldtag

cd "${ROOT}"

if [ -e go-new ] ; then
	rm -rf go-new
fi
git clone https://go.googlesource.com/go go-new
if [ $? != 0 ] ; then
	echo git clone go-new failed
	exit 1
fi
cd go-new/src

git fetch "https://go.googlesource.com/go" "${cl}"
if [ $? != 0 ] ; then
	echo git fetch "https://go.googlesource.com/go" "${cl}"  failed
	exit 1
fi
git checkout FETCH_HEAD
if [ $? != 0 ] ; then
	echo git checkout FETCH_HEAD failed
	exit 1
fi

./make.bash
if [ $? != 0 ] ; then
	echo make.bash failed
	exit 1
fi
newtag=`git log -n 1 --format='%h'`
export newtag

cd "${ROOT}"
${PERFLOCK} bent -v -N=${N} -a=${B} -L=bentjobs.log -C=configurations-cmpjob.toml "$@"
RUN=`tail -1 bentjobs.log | awk -c '{print $1}'`

cd bench
STAMP="stamp-$$"
export STAMP
echo "suite: bent-cmp-cl" >> ${STAMP}
echo "bentstamp: ${RUN}" >> "${STAMP}"
echo "oldtag: ${oldtag}" >> "${STAMP}"
echo "newtag: ${newtag}" >> "${STAMP}"

oldlog="old-${oldtag}"
newlog="new-${newtag}"

cat ${RUN}.Old.build > ${oldlog}
cat ${RUN}.New.build > ${newlog}
grep -E '^(Benchmark|[-_a-zA-Z0-9]+:)' ${RUN}.Old.stdout >> ${oldlog}
grep -E '^(Benchmark|[-_a-zA-Z0-9]+:)' ${RUN}.New.stdout >> ${newlog}
cat ${RUN}.Old.{benchsize,benchdwarf} >> ${oldlog}
cat ${RUN}.New.{benchsize,benchdwarf} >> ${newlog}
benchsave -header "${STAMP}" "${oldlog}" "${newlog}"
rm "${STAMP}"
