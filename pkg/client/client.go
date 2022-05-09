package client

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"

	clusterv1alpha1 "github.com/karmada-io/karmada/pkg/apis/cluster/v1alpha1"
	karmadaclientset "github.com/karmada-io/karmada/pkg/generated/clientset/versioned"
	helmclient "helm.sh/helm/v3/pkg/kube"
	crdclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Client struct {
	kube kubeclient.Interface
	crd  crdclientset.Interface
	helm helmclient.Interface

	karmada karmadaclientset.Interface
}

func NewClient(rest genericclioptions.RESTClientGetter) (*Client, error) {
	c, err := rest.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	kubeClient := kubeclient.NewForConfigOrDie(c)
	helmClient := helmclient.New(rest)
	crdClientSet := crdclientset.NewForConfigOrDie(c)
	karmadaClient := karmadaclientset.NewForConfigOrDie(c)

	return &Client{
		kube:    kubeClient,
		helm:    helmClient,
		crd:     crdClientSet,
		karmada: karmadaClient,
	}, nil
}

func (c *Client) KubeClient() kubeclient.Interface {
	return c.kube
}

func (c *Client) KarmadaClient() karmadaclientset.Interface {
	return c.karmada
}

func (c *Client) CrdClient() crdclientset.Interface {
	return c.crd
}

func (c *Client) HelmClient() helmclient.Interface {
	return c.helm
}

// Copied from karmada, because we donot want to build the controller-runtime client.
func (c *Client) memberClusterConfig(clusterName string) (*rest.Config, error) {
	cluster, err := c.karmada.ClusterV1alpha1().Clusters().Get(context.TODO(), clusterName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	apiEndpoint := cluster.Spec.APIEndpoint
	if apiEndpoint == "" {
		return nil, fmt.Errorf("the api endpoint of cluster %s is empty", clusterName)
	}

	secretNamespace := cluster.Spec.SecretRef.Namespace
	secretName := cluster.Spec.SecretRef.Name
	if secretName == "" {
		return nil, fmt.Errorf("cluster %s does not have a secret name", clusterName)
	}
	secret, err := c.kube.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	token, tokenFound := secret.Data[clusterv1alpha1.SecretTokenKey]
	if !tokenFound || len(token) == 0 {
		return nil, fmt.Errorf("the secret for cluster %s is missing a non-empty value for %q", clusterName, clusterv1alpha1.SecretTokenKey)
	}

	clusterConfig, err := clientcmd.BuildConfigFromFlags(apiEndpoint, "")
	if err != nil {
		return nil, err
	}

	clusterConfig.BearerToken = string(token)

	if cluster.Spec.InsecureSkipTLSVerification {
		clusterConfig.TLSClientConfig.Insecure = true
	} else {
		clusterConfig.CAData = secret.Data[clusterv1alpha1.SecretCADataKey]
	}

	if cluster.Spec.ProxyURL != "" {
		proxy, err := url.Parse(cluster.Spec.ProxyURL)
		if err != nil {
			log.Printf("parse proxy error. %v\n", err)
			return nil, err
		}
		clusterConfig.Proxy = http.ProxyURL(proxy)
	}

	return clusterConfig, nil
}

func (c *Client) NewClusterClientSet(clusterName string) (kubeclient.Interface, error) {
	clusterConfig, err := c.memberClusterConfig(clusterName)
	if err != nil {
		return nil, err
	}
	return kubeclient.NewForConfig(clusterConfig)
}