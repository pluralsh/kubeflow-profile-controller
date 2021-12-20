module github.com/pluralsh/kubeflow-profile-controller

go 1.16

require (
	github.com/aws/aws-sdk-go v1.41.16
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/crossplane/crossplane-runtime v0.15.1-0.20210930095326-d5661210733b
	github.com/crossplane/provider-aws v0.22.0
	github.com/fsnotify/fsnotify v1.5.1
	// github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-logr/logr v0.4.0
	github.com/goccy/go-yaml v1.9.4
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.17.0
	github.com/pkg/errors v0.9.1
	github.com/pluralsh/controller-reconcile-helper v0.0.0-20211220162507-538e238050b0
	github.com/prometheus/client_golang v1.11.0
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/tidwall/gjson v1.11.0
	golang.org/x/oauth2 v0.0.0-20210628180205-a41e5a781914
	google.golang.org/api v0.50.0
	istio.io/api v0.0.0-20211122230647-4866a573a9cb
	istio.io/client-go v1.12.0
	k8s.io/api v0.22.4
	k8s.io/apimachinery v0.22.4
	k8s.io/client-go v0.22.4
	sigs.k8s.io/controller-runtime v0.10.3
	sigs.k8s.io/yaml v1.3.0
)
