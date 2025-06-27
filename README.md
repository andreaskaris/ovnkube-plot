A lot of the code is heavily borrowed from the ovndbchecker from https://github.com/ovn-org/ovn-kubernetes

## Build Instructions

~~~
make
~~~

## Usage Instructions

~~~
NAME:
   ovnkube-plot - plot ovnkube network in a human readable way

USAGE:
   ovnkube-plot [global options]

VERSION:
   0.0.1

COMMANDS:
     help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   OVN KUBEPLOT OPTIONS
   --format value  The output format ('compact' or 'detailed') (default: "compact")
   --filter value  Show only matching nodes (default: ".*")
   --mode value    The mode to use ('auto', 'ovn-kubernetes', or 'ovn') (default: "auto")
   
   OVN NORTHBOUND DB OPTIONS
   --nb-address value              IP address and port of the OVN northbound API (eg, ssl:1.2.3.4:6641,ssl:1.2.3.5:6642).  Leave empty to use a local unix socket.
   --nb-client-privkey value       Private key that the client should use for talking to the OVN database (default when ssl address is used: /etc/openvswitch/ovnnb-privkey.pem).  Default value for this setting is empty which defaults to use local unix socket.
   --nb-client-cert value          Client certificate that the client should use for talking to the OVN database (default when ssl address is used: /etc/openvswitch/ovnnb-cert.pem). Default value for this setting is empty which defaults to use local unix socket.
   --nb-client-cacert value        CA certificate that the client should use for talking to the OVN database (default when ssl address is used: /etc/openvswitch/ovnnb-ca.cert).Default value for this setting is empty which defaults to use local unix socket.
   --nb-cert-common-name value     Common Name of the certificate used for TLS server certificate verification. In cases where the certificate doesn't have any SAN Extensions, this parameter should match the DNS(hostname) of the server. In case the certificate has a SAN extension, this parameter should match one of the SAN fields.
   --nb-raft-election-timer value  The desired northbound database election timer. (default: 0)
~~~

Example:
~~~
# make plot
bin/ovnkube-plot --filter=".*" --mode auto --nb-address=tcp://172.18.0.3:6641,tcp://172.18.0.2:6641,tcp://172.18.0.4:6641  --sb-address=tcp://172.18.0.3:6642,tcp://172.18.0.2:6642,tcp://172.18.0.4:6642  > output/compact.txt
I0627 17:09:37.248998  304063 ovs.go:98] Maximum command line arguments set to: 191102
I0627 17:09:37.256058  304063 go_ovn.go:165] Created OVNDB TCP client for db: OVN_Northbound
I0627 17:09:37.256081  304063 go_ovn.go:41] Created OVN NB client with Scheme: tcp
cat output/compact.txt | dot -Tpdf > output/compact.pdf
bin/ovnkube-plot --filter=".*" --mode auto --format detailed --nb-address=tcp://172.18.0.3:6641,tcp://172.18.0.2:6641,tcp://172.18.0.4:6641  --sb-address=tcp://172.18.0.3:6642,tcp://172.18.0.2:6642,tcp://172.18.0.4:6642  > output/detailed.txt
I0627 17:09:37.359020  304093 ovs.go:98] Maximum command line arguments set to: 191102
I0627 17:09:37.366110  304093 go_ovn.go:165] Created OVNDB TCP client for db: OVN_Northbound
I0627 17:09:37.366140  304093 go_ovn.go:41] Created OVN NB client with Scheme: tcp
cat output/detailed.txt | dot -Tpdf > output/detailed.pdf
~~~
