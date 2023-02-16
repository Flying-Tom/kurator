---
title: "Deploy Cluster on AWS"
linkTitle: "Deploy Cluster on AWS"
description: >
  The easiest way to deploy cluster on AWS with Kurator.
---

In this tutorial we’ll cover the basics of how to use [Cluster API](https://cluster-api.sigs.k8s.io) and kurator cluster operator to create one or more Kubernetes clusters.

## Prerequisites

### Setup kubernetes clusters with Kind

Deploy a kubernetes cluster using kurator's scripts.

```bash
git clone https://github.com/kurator-dev/kurator.git
cd kurator
hack/local-setup-cluster.sh
```

### Install cert manager

Kurator cluster operator depends on [cert manager CA injector](https://cert-manager.io/docs/concepts/ca-injector).

***Please make sure cert manager is ready before install cluster operator***

```console
helm repo add jetstack https://charts.jetstack.io
helm repo update
kubectl create namespace cert-manager
helm install -n cert-manager cert-manager jetstack/cert-manager --set installCRDs=true
```

## Build `cluster-operator` from source

Build cluster operator image:

```console
make docker
```

Build cluster operator helm chart:

```console
make gen-chart
```

Load image to kind cluster:

```console
kind load docker-image ghcr.io/kurator-dev/cluster-operator:latest --name kurator
```

## Prepare AWS Credentials

Download the latest binary of clusterawsadm from the [AWS provider releases](https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases/tag/v2.0.0). The clusterawsadm command line utility assists with identity and access management (IAM) for [Cluster API Provider AWS](https://cluster-api-aws.sigs.k8s.io/).

```console
# Download the latest release
curl -L https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases/download/v2.0.0/clusterawsadm-linux-amd64 -o clusterawsadm
# Make it executable
chmod +x clusterawsadm
# Move the binary to a directory present in your PATH
sudo mv clusterawsadm /usr/local/bin
# Check version to confirm installation
clusterawsadm version

export AWS_REGION=us-east-1 # This is used to help encode your environment variables
# Get AK/SK from https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html
export AWS_ACCESS_KEY_ID=<your-access-key>
export AWS_SECRET_ACCESS_KEY=<your-secret-access-key>

# The clusterawsadm utility takes the credentials that you set as environment
# variables and uses them to create a CloudFormation stack in your AWS account
# with the correct IAM resources.
clusterawsadm bootstrap iam create-cloudformation-stack

# Create the base64 encoded credentials using clusterawsadm.
# This command uses your environment variables and encodes
# them in a value to be stored in a Kubernetes Secret.
export AWS_B64ENCODED_CREDENTIALS=$(clusterawsadm bootstrap credentials encode-as-profile)
```

## Install cluster operator

```console
# Please make sure cert manager is ready before install cluster operator
kubectl create namespace kurator-system
helm install kurator-base out/charts/base-0.1.0.tgz -n kurator-system
helm install -n kurator-system kurator-cluster-operator out/charts/cluster-operator-0.1.0.tgz --set aws.credentials=${AWS_B64ENCODED_CREDENTIALS}
```

## Create a vanilla cluster on AWS

The clusterctl CLI tool handles the lifecycle of a Cluster API management cluster.

```console
# download clusterctl
curl -L https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.2.5/clusterctl-linux-amd64 -o clusterctl
sudo install -o root -g root -m 0755 clusterctl /usr/local/bin/clusterctl
# verify version
clusterctl version
```

Create a kubernetes cluster on AWS, which contains control plane nodes and worker nodes, the cluster topology shows as follows:

{{< image width="75%"
    link="./image/capa-aws.png"
    >}}

Generating the cluster configuration:

```console
export AWS_CONTROL_PLANE_MACHINE_TYPE=t3.large
export AWS_NODE_MACHINE_TYPE=t3.large
export AWS_REGION=us-east-1
export AWS_SSH_KEY_NAME=default
export KUBERNETES_VERSION=v1.25.0
export CONTROL_PLANE_MACHINE_COUNT=3
export WORKER_MACHINE_COUNT=3
clusterctl generate cluster capi-quickstart --infrastructure=aws:v2.0.0 > manifests/examples/capi-quickstart.yaml
```

The cluster resource topology shows as follows:

{{< image width="75%"
    link="./image/capa-crd.png"
    >}}


Apply the cluster manifest:

```console
kubectl apply -f manifests/examples/capi-quickstart.yaml
```

> if you want create a cluster with multi instance types, please checkout the [multi nodes demo](https://github.com/kurator-dev/kurator/blob/main/manifests/examples/multi-tenancy/capi-nodes.yaml)

Wait the control plane is up:

```console
kubectl get kubeadmcontrolplane -w
```

***The control plane won’t be Ready until we install a CNI in the next step.***

```console
# retrieve the cluster Kubeconfig 
clusterctl get kubeconfig capi-quickstart > /root/.kube/capi-quickstart.kubeconfig
# deploy calico solution
kubectl --kubeconfig=/root/.kube/capi-quickstart.kubeconfig apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.24.1/manifests/calico.yaml
# After a short while, our nodes should be running and in Ready state
kubectl --kubeconfig=/root/.kube/capi-quickstart.kubeconfig get nodes
```

## Cleanup

***IMPORTANT: In order to ensure a proper cleanup of your infrastructure you must always delete the cluster object. Deleting the entire cluster template with kubectl delete -f capi-quickstart.yaml might lead to pending resources to be cleaned up manually.***

```console
kubectl delete cluster capi-quickstart
```

Uninstall cluster operator:

```console
helm uninstall kurator-cluster-operator -n kurator-system 
helm uninstall kurator-base -n kurator-system
kubectl delete ns kurator-system
```

*Optional*, unintall cert manager:

```console
helm uninstall -n cert-manager cert-manager
```
