
ssh root@esx.gw.lo "cd /vmfs/volumes/datastore1/$1;rm $1.vmdk;rm $1.vmsd;rm $1-flat.vmdk;vmkfstools --createvirtualdisk 160G --diskformat thin $1.vmdk"

