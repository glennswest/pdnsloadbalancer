#!/bin/sh
set -e

clusterdir=~/devel/installer/cluster0/
config=~/devel/installer/initial/install-config.yaml

version=$(curl -s https://openshift-release.svc.ci.openshift.org/api/v1/releasestream/4.2.0-0.ci/latest | jq '.pullSpec')
echo $version
version=${version%\"}
echo $version
version=${version#*:}
echo "Using version $version"


oc adm release extract --tools registry.svc.ci.openshift.org/ocp/release:$version -a ./dev.secret
# tar -xf openshift-install-linux-$version.tar.gz
# rm *.gz
oc adm release extract --command=openshift-install registry.svc.ci.openshift.org/ocp/release:$version || true

#if [ "$1" = "-d" ]; then
#	./openshift-install --dir=$clusterdir destroy cluster
#fi

# rm -rf $clusterdir/*
#cp $config $clusterdir

#./openshift-install --dir=$clusterdir create cluster

#popd
