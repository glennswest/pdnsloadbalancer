rm -r -f gw
mkdir gw
cp install-config.yaml gw
openshift-install create ignition-configs --dir=gw
scp gw/* root@store.gw.lo:/volume1/tftp
./poweroff-all-vms.sh
sleep 5
./erase-all-vms.sh
sleep 5
./poweron-all-vms.sh
openshift-install --dir=gw wait-for bootstrap-complete --log-level debug
openshift-install --dir=gw wait-for install-complete --log-level debug





