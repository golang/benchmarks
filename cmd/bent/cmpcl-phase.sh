#!/bin/bash -x

# Variant of cmpcl.sh for comparing SSA phase timings for a CL and its immediate predecessor.

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
N=0
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

STAMP="$$"
export STAMP

cd "${ROOT}"
${PERFLOCK} bent -U -v -N=${N} -a=${B} -L=bentjobs.log -c=Old-phase,New-phase -C=configurations-cmpjob.toml "$@" | tee phases.${STAMP}.log
phase-times > phases.${STAMP}.csv

