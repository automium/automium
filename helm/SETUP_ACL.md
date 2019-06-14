## Setup Consul with ACLs

Once helm is ready, you can install Consul using the [official chart](https://github.com/hashicorp/consul-helm). Since the project doesn't use a real Helm repository, you need to install it from GitHub, using some custom values.

Before running Helm, you need to add a local volume, because Consul pod remains pending until a data store is present. So, run the following command: `kubectl apply -f consul/`.

`helm install --name consul -f consul-values.yaml --set global.bootstrapACLs=true https://github.com/hashicorp/consul-helm/archive/v0.8.1.tar.gz`

The Helm chart provides a valid Consul ACL Token in this K8s secret:

`kubectl get secrets consul-consul-bootstrap-acl-token -o 'go-template={{index .data "token"}}'`.

Decode the base64 token before use it in Consul.

### Configure Automium playground

Everything it's needed for getting started with Automium could be installed running Helm against the directory "automium":

`helm install --name automium ./automium`

Setup consists mainly on running what's required for playing around and test Automium. So, to be clear, this chart is not intended for running Automium in a production scenario.

In order to view the provided dashboards you have to map the urls in your local _etc/hosts_ file, adding this two lines:

```
127.0.0.1    consul.automium.local
127.0.0.1    lb.automium.local
```

Open the url http://consul.automium.local:8081 to check Consul dashboard is working and try the token in the ACL section.

To enable the ACLs in all the environment, you need to set the token in the Automium example app, editing the example statefulset (`kubectl edit statefulset example`) with the following changes:  

From:
```
spec:
      containers:
      - name: example
```

To: 
```
spec:
      containers:
      - name: example
        env:
        - name: CONSUL_HTTP_TOKEN
          value: TOKEN-VALUE
```

### Next

Follow the main guide [here](https://github.com/automium/automium/blob/master/helm/SETUP.md#run-automium) to run and test Automium.
Just remember to use the acl version of the example service!