#!/bin/bash -x

# perflock is not always available
PERFLOCK=`which perflock`

${PERFLOCK} echo "Gratuitous perflock to prevent this script from starting if something else is still in progress"

ROOT="${HOME}/work/bent-cron"
export ROOT
cd "${ROOT}"

# BASE is the baseline, defined here, assumed checked out and built.
BASE=Go1.17
export BASE

# N is number of benchmarks, B is number of builds
# Can override these with -N= and -a= on command line, or in localfix.
#
N=25
B=25
NNl=0
BNl=1
Nl=0
Bl=1

# Adjust N, B, define NUMACTL, set GOMAXPROCS, as necessary.
if [ -e ./localfix ] ; then
	. ./localfix
fi

if [ "x${SUITE}" = "x" ] ; then
	SUITE="bent-cron"
fi

if [ ! -e "${BASE}" ] ; then
	echo Missing expected baseline directory "${BASE}" in "${ROOT}", attempting to checkout and build.
	base=`echo $BASE | tr G g`
	${PERFLOCK} git clone https://go.googlesource.com/go -b release-branch.${base} ${BASE}
	if [ $? != 0 ] ; then
		echo git clone https://go.googlesource.com/go -b release-branch.${base} ${BASE} FAILED
		exit 1
	fi
	cd ${BASE}/src
	${PERFLOCK} ./make.bash
	if [ $? != 0 ] ; then
		echo BASE make.bash FAILED
		exit 1
	fi
	cd "${ROOT}"
fi

# Refresh tip, get revision
if [ -e go-tip ] ; then
	${PERFLOCK} rm -rf go-tip
fi
${PERFLOCK} git clone https://go.googlesource.com/go go-tip
if [ $? != 0 ] ; then
	echo git clone go-tip failed
	exit 1
fi
cd go-tip/src
${PERFLOCK} ./make.bash
if [ $? != 0 ] ; then
	echo make.bash failed
	exit 1
fi
tip=`git log -n 1 --format='%h'`
tiptime=`git log -n 1 --format='%cI'`

# Get revision for base so there is no ambiguity
cd "${ROOT}"/${BASE}
base=`git log -n 1 --format='%h'`

# Optimized build and benchmark

cd "${ROOT}"
# For arm64 big.little, might need to prefix with something like:
# GOMAXPROCS=4 numactl -C 2-5 -- ...
GOARCH="${BENTARCH}" ${NUMACTL} ${PERFLOCK} bent -v -N=${N} -a=${B} -L=bentjobs.log -C=configurations-cronjob.toml -c Base,Tip "$@"
RUN=`tail -1 bentjobs.log | awk -c '{print $1}'`
runstamp="$RUN"
bentstamp="$RUN"

# variables for better benchmarking
denominator_branch="${BASE}"
denominator_hash="${base}"

numerator_branch="master"
numerator_hash="$tip"
numerator_stamp="$tiptime"

builder_id=`uname -n`
builder_type="${BUILDER_TYPE}"

cd bench
STAMP="stamp-$$"
export STAMP

append () {
    c=`eval echo $\`echo $1\``
	echo "$1: $c" >> ${STAMP}
}

echo "suite: ${SUITE}" >> ${STAMP}
append bentstamp
append tip
append base
# new stuff for better benchmarking
append numerator_branch
append numerator_hash
append numerator_stamp
append denominator_branch
append denominator_hash
append builder_id
append builder_type
append runstamp

SFX="${RUN}"

cp ${STAMP} ${BASE}-opt.${SFX}
cp ${STAMP} ${tip}-opt.${SFX}

cat ${RUN}.Base.build >> ${BASE}-opt.${SFX}
cat ${RUN}.Tip.build >> ${tip}-opt.${SFX}
egrep '^(Benchmark|[-_a-zA-Z0-9]+:)' ${RUN}.Base.stdout >> ${BASE}-opt.${SFX}
egrep '^(Benchmark|[-_a-zA-Z0-9]+:)' ${RUN}.Tip.stdout >> ${tip}-opt.${SFX}
cat ${RUN}.Base.{benchsize,benchdwarf} >> ${BASE}-opt.${SFX}
cat ${RUN}.Tip.{benchsize,benchdwarf} >> ${tip}-opt.${SFX}
benchsave ${BASE}-opt.${SFX} ${tip}-opt.${SFX}
rm "${STAMP}"

cd ${ROOT}
# The following depends on some other infrastructure, see:
#     github/com/dr2chase/go-bench-tweet-bot
#     https://go-review.googlesource.com/c/perf/+/218923 (benchseries)
if [ -e ./tweet-results ] ; then
	./tweet-results "${RUN}"
fi

# Debugging build

cd "${ROOT}"
GOARCH="${BENTARCH}" ${NUMACTL} ${PERFLOCK} bent -v -N=${NNl} -a=${BNl} -L=bentjobsNl.log -C=configurations-cronjob.toml -c BaseNl,TipNl
RUN=`tail -1 bentjobsNl.log | awk -c '{print $1}'`
runstamp="$RUN"
bentstamp="$RUN"

cd bench
STAMP="stamp-$$"
export STAMP

echo "suite: ${SUITE}-Nl" >> ${STAMP}

append bentstamp
append tip
append base
# new stuff for better benchmarking
append numerator_branch
append numerator_hash
append numerator_stamp
append denominator_branch
append denominator_hash
append builder_id
append builder_type
append runstamp

SFX="${RUN}"

cp ${STAMP} ${BASE}-Nl.${SFX}
cp ${STAMP} ${tip}-Nl.${SFX}

cat ${RUN}.BaseNl.build >> ${BASE}-Nl.${SFX}
cat ${RUN}.TipNl.build >> ${tip}-Nl.${SFX}
egrep '^(Benchmark|[-_a-zA-Z0-9]+:)' ${RUN}.BaseNl.stdout >> ${BASE}-Nl.${SFX}
egrep '^(Benchmark|[-_a-zA-Z0-9]+:)' ${RUN}.TipNl.stdout >> ${tip}-Nl.${SFX}
cat ${RUN}.BaseNl.{benchsize,benchdwarf} >> ${BASE}-Nl.${SFX}
cat ${RUN}.TipNl.{benchsize,benchdwarf} >> ${tip}-Nl.${SFX}
benchsave ${BASE}-Nl.${SFX} ${tip}-Nl.${SFX}
rm "${STAMP}"

# No-inline build

cd "${ROOT}"
${NUMACTL} ${PERFLOCK} bent -v -N=${Nl} -a=${Bl} -L=bentjobsl.log -C=configurations-cronjob.toml -c Basel,Tipl
RUN=`tail -1 bentjobsl.log | awk -c '{print $1}'`
runstamp="$RUN"
bentstamp="$RUN"

cd bench
STAMP="stamp-$$"
export STAMP
echo "suite: ${SUITE}-l" >> ${STAMP}
append bentstamp
append tip
append base
# new stuff for better benchmarking
append numerator_branch
append numerator_hash
append numerator_stamp
append denominator_branch
append denominator_hash
append builder_id
append builder_type
append runstamp

SFX="${RUN}"

cp ${STAMP} ${BASE}-l.${SFX}
cp ${STAMP} ${tip}-l.${SFX}

cat ${RUN}.Basel.build >> ${BASE}-l.${SFX}
cat ${RUN}.Tipl.build >> ${tip}-l.${SFX}
egrep '^(Benchmark|[-_a-zA-Z0-9]+:)' ${RUN}.Basel.stdout >> ${BASE}-l.${SFX}
egrep '^(Benchmark|[-_a-zA-Z0-9]+:)' ${RUN}.Tipl.stdout >> ${tip}-l.${SFX}
cat ${RUN}.Basel.{benchsize,benchdwarf} >> ${BASE}-l.${SFX}
cat ${RUN}.Tipl.{benchsize,benchdwarf} >> ${tip}-l.${SFX}
benchsave ${BASE}-l.${SFX} ${tip}-l.${SFX}
rm "${STAMP}"
