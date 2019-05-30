# Setting up Automium 

## Components

**Kubernetes**  
Automium has been developed as a Kubernetes Operator with [Kubebuilder](https://github.com/kubernetes-sigs/kubebuilder).

**Consul**  
Each service node (VM, Cloud Instance, Container) managed by Automium is registered in [Consul](https://www.consul.io/). Consul provides _service discovery_ and _health checks_.

**Provisioner**  
Our “secret recipe” to setup a service. It is defined as a collection of Terraform scripts and Ansible roles. Each service (Kubernetes, HAProxy, …) has a specific provisioner based on this [base project](https://github.com/automium/provisioner). 

## Standalone Installation

To setup the required components in a test environment we can use [k3s](https://github.com/rancher/k3s) instead of a full Kubernetes cluster, a single Consul server running in k3s and an example provisioner.

Follow the tutorial here for a complete description on how to run k3s using the related project [k3d](https://github.com/rancher/k3d). After the setup of K3d, just run this command to create the default cluster:

`k3d create --publish 8081:80`

To check the k3s cluster is up and running, you can check the cluster node is ready with the commands:

```
export KUBECONFIG="$(k3d get-kubeconfig --name='k3s-default')"
kubectl get nodes
```

We use [Helm](https://helm.sh/) to setup a Consul server in k3s. Follow this [tutorial](https://helm.sh/docs/using_helm/#installing-helm) to install the client and then run the following commands to complete the setup (be sure to move to the /test folder of the repo!):  

```  
kubectl apply -f rbac-config.yaml
helm init --upgrade --service-account tiller
```

Once helm is ready, we can install Consul using the [official chart](https://github.com/hashicorp/consul-helm):

`helm install --name consul -f consul/values.yaml https://github.com/hashicorp/consul-helm/archive/v0.8.1.tar.gz`

Consul pod remains pending until we define a local volume to store data, with the following commands:

```
kubectl apply -f consul/pv.yaml
kubectl apply -f consul/storageclass.yaml
```

In order to view the Consul dashboard, and later to publish a load balancer for our own services, we run a Kubernetes Ingress:

`kubectl apply -f ingress.yaml`

The ingress matches 2 different domains. You have to map them in your local _etc/hosts_ file, adding this two lines:

```
127.0.0.1    consul.automium.local
127.0.0.1    lb.automium.local
```

Eventually, open the url http://consul.automium.local:8081 to check Consul dashboard is working.

The load balancer is not running yet. Let's start it with the following commands:

```
kubectl create configmap lb-config --from-file=lb/haproxy.cfg
kubectl apply -f lb/
```

Try to open the url http://lb.automium.local:8081/stats to check the HAProxy stats page.

### Run Automium

At this point, we have all the required components for Automium Operator.
Before deploying the CRDs and run the manager, you need to create a configmap with the default configuration:

`kubectl create configmap provisioner-config`

From the root of the repo, run the following commands to deploy the custom CRDs in the k3s cluster and run the Automium controllers locally, pointing the running consul API:

```
make install
CONSUL_ADDRESS=consul.automium.local:8081 make run
```

Automium is now up and running. Let's move to another terminal to interact with it.  

### Try the example provisioner
We are going to run the example service, that's a custom application built for demo and test purposes. Look at the project [here](todo_add_link) if you want to understand the application or contribute to improve it!

Automium Operator runs the example service trough a [specific provisioner](https://github.com/automium/automium/tree/master/test/provisioner) that knows how to deploy, scale, upgrade and delete the application.

Before run the service, we need to configure the provisioner adding the role to manage  pods in the default namespace. This can be done applying these 2 yaml files from the root folder:

```
kubectl apply -f test/provisioner/role.yaml
kubectl apply -f test/provisioner/rolebinding.yaml
```

It's time to create an example service applying this specs with kubectl:

`kubectl apply -f config/samples/core_v1beta1_example-service.yaml`

The [service controller](https://github.com/automium/automium/blob/master/pkg/controller/service/service_controller.go) will be triggered to launch the example provisioner. Running `kubectl get service.core` you should see this output:

```
NAME               APPLICATION   VERSION   REPLICAS   FLAVOR          MODULE                    STATUS    AGE
automium-example   example       1.0.0     1          e3standard.x1   automium-example-module   Running   2s
```

To double-check the example service is up and running, you can open the url http://lb.automium.local:8081 or check the load balancer stats page.

### Cleanup

To destroy the test environment, simply run `k3d delete`.
