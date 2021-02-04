#!/bin/bash -x

if [ $# -lt 2 ] ; then
  echo cmpjob.sh "<branch-or-tag>" "<branch-or-tag>" "bent-options"
  exit 1
fi

if [ ${1:0:1} = "-" ] ; then
  echo "First parameter should be a git tag or branch"
  exit 1
fi

if [ ${2:0:1} = "-" ] ; then
  echo "Second parameter should be a git tag or branch"
  exit 1
fi

oldtag="$1"
shift

newtag="$1"
shift

ROOT=`pwd`
export ROOT oldtag newtag

# perflock is not always available
PERFLOCK=`which perflock`

# N is number of benchmarks, B is number of builds
# Can override these with -N= and -a= on command line.
N=15
B=1

REPO="https://go.googlesource.com/go"

cd "${ROOT}"

if [ -e go-old ] ; then
	rm -rf go-old
fi

git clone "${REPO}" go-old
if [ "$?" != "0" ] ; then
	echo git clone "${REPO}" go-old FAILED
	exit 1
fi
cd go-old/src
git fetch "${REPO}" "${oldtag}"
git checkout "${oldtag}"
if [ $? != 0 ] ; then
	echo git checkout "${oldtag}" failed
	exit 1
fi
./make.bash
if [ $? != 0 ] ; then
	echo BASE make.bash FAILED
	exit 1
fi

cd "${ROOT}"

if [ -e go-new ] ; then
	rm -rf go-new
fi
git clone "${REPO}" go-new
if [ $? != 0 ] ; then
	echo git clone go-new failed
	exit 1
fi
cd go-new/src
git fetch "${REPO}" "${newtag}"
git checkout "${newtag}"
if [ $? != 0 ] ; then
	echo git checkout "${newtag}" failed
	exit 1
fi
./make.bash
if [ $? != 0 ] ; then
	echo make.bash failed
	exit 1
fi

cd "${ROOT}"
${PERFLOCK} bent -U -v -N=${N} -a=${B} -L=bentjobs.log -C=configurations-cmpjob.toml "$@"
RUN=`tail -1 bentjobs.log | awk -c '{print $1}'`

cd bench
STAMP="stamp-$$"
export STAMP
echo "suite: bent-cmp-branch" >> "${STAMP}"
echo "bentstamp: ${RUN}" >> "${STAMP}"
echo "oldtag: ${oldtag}" >> "${STAMP}"
echo "newtag: ${newtag}" >> "${STAMP}"

oldlog="old-${oldtag}"
newlog="new-${newtag}"

cat "${RUN}.Old.build" > "${oldlog}"
cat "${RUN}.New.build" > "${newlog}"
egrep '^(Benchmark|[-_a-zA-Z0-9]+:)' "${RUN}.Old.stdout" >> "${oldlog}"
egrep '^(Benchmark|[-_a-zA-Z0-9]+:)' "${RUN}.New.stdout" >> "${newlog}"
cat "${RUN}.Old.{benchsize,benchdwarf}" >> "${oldlog}"
cat "${RUN}.New.{benchsize,benchdwarf}" >> "${newlog}"
benchsave -header "${STAMP}" "${oldlog}" "${newlog}"
rm "${STAMP}"
