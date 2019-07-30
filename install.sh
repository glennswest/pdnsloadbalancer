#!/bin/sh
set -e

clusterdir=~/devel/installer/cluster0/
config=~/devel/installer/initial/install-config.yaml

oc login https://api.ci.openshift.org --token=lAnqAwvaNgTLJWabkLo-do211o5NZEQeHjvG2JBjHHo
oc registry login
version=$(curl -s https://openshift-release.svc.ci.openshift.org/api/v1/releasestream/4.2.0-0.ci/latest | jq '.pullSpec')
echo $version
version=${version%\"}
echo $version
version=${version#*:}
echo "Using version $version"

rm -r -f wip
mkdir wip
cd wip
oc adm release extract --tools registry.svc.ci.openshift.org/ocp/release:$version 
ls -l
tar -xf openshift-install-mac-$version.tar.gz
cp openshift-install /usr/local/sbin
rm *.gz
cd ..
./run.sh
