A lot of the code is heavily borrowed from the ovndbchecker from https://github.com/ovn-org/ovn-kubernetes

## Build Instructions

~~~
make
~~~

## Usage Instructions

~~~
./ovnkube-plot  --nb-address=tcp://172.18.0.3:6641,tcp://172.18.0.2:6641,tcp://172.18.0.4:6641 --sb-address=tcp://172.18.0.3:6642,tcp://172.18.0.2:6642,tcp://172.18.0.4:6642 > plot.txt
cat dot.txt | dot -Tpdf > plot.pdf
evince plot.pdf
~~~
