package reconcile

import (
	"context"
	"reflect"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/go-logr/logr"

	// istioNetworking "istio.io/api/networking/v1beta1"
	istioNetworkingClient "istio.io/client-go/pkg/apis/networking/v1beta1"
	// istioSecurity "istio.io/api/security/v1beta1"
	crossplaneAWSIdentity "github.com/crossplane/provider-aws/apis/identity/v1alpha1"
	istioSecurityClient "istio.io/client-go/pkg/apis/security/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	hncv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

// Deployment reconciles a k8s deployment object.
func Deployment(ctx context.Context, r client.Client, deployment *appsv1.Deployment, log logr.Logger) error {
	foundDeployment := &appsv1.Deployment{}
	justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating Deployment", "namespace", deployment.Namespace, "name", deployment.Name)
			if err := r.Create(ctx, deployment); err != nil {
				log.Error(err, "unable to create deployment")
				return err
			}
			justCreated = true
		} else {
			log.Error(err, "error getting deployment")
			return err
		}
	}
	if !justCreated && CopyDeploymentSetFields(deployment, foundDeployment) {
		log.Info("Updating Deployment", "namespace", deployment.Namespace, "name", deployment.Name)
		if err := r.Update(ctx, foundDeployment); err != nil {
			log.Error(err, "unable to update deployment")
			return err
		}
	}

	return nil
}

// Service reconciles a k8s service object.
func Service(ctx context.Context, r client.Client, service *corev1.Service, log logr.Logger) error {
	foundService := &corev1.Service{}
	justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating Service", "namespace", service.Namespace, "name", service.Name)
			if err = r.Create(ctx, service); err != nil {
				log.Error(err, "unable to create service")
				return err
			}
			justCreated = true
		} else {
			log.Error(err, "error getting service")
			return err
		}
	}
	if !justCreated && CopyServiceFields(service, foundService) {
		log.Info("Updating Service\n", "namespace", service.Namespace, "name", service.Name)
		if err := r.Update(ctx, foundService); err != nil {
			log.Error(err, "unable to update Service")
			return err
		}
	}

	return nil
}

// VirtualService reconciles an Istio virtual service object.
func VirtualService(ctx context.Context, r client.Client, virtualservice *istioNetworkingClient.VirtualService, log logr.Logger) error {
	foundVirtualService := &istioNetworkingClient.VirtualService{}
	// justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: virtualservice.Name, Namespace: virtualservice.Namespace}, foundVirtualService); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating virtual service", "namespace", virtualservice.Namespace, "name", virtualservice.Name)
			if err := r.Create(ctx, virtualservice); err != nil {
				log.Error(err, "unable to create virtual service")
				return err
			}
			// justCreated = true
		} else {
			log.Error(err, "error getting virtual service")
			return err
		}
	} else {
		if CopyVirtualService(virtualservice, foundVirtualService) {
			log.Info("Updating virtual service", "namespace", virtualservice.Namespace, "name", virtualservice.Name)
			if err := r.Update(ctx, foundVirtualService); err != nil {
				log.Error(err, "unable to update virtual service")
				return err
			}
		}
	}
	return nil
}

// Namespace reconciles a Namespace object.
func Namespace(ctx context.Context, r client.Client, namespace *corev1.Namespace, log logr.Logger) error {
	foundNamespace := &corev1.Namespace{}
	justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: namespace.Name, Namespace: namespace.Namespace}, foundNamespace); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating namespace", "namespace", namespace.Name)
			if err = r.Create(ctx, namespace); err != nil {
				// IncRequestErrorCounter("error creating namespace", SEVERITY_MAJOR)
				log.Error(err, "Unable to create namespace")
				return err
			}
			err = backoff.Retry(
				func() error {
					return r.Get(ctx, types.NamespacedName{Name: namespace.Name}, foundNamespace)
				},
				backoff.WithMaxRetries(backoff.NewConstantBackOff(3*time.Second), 5))
			if err != nil {
				// IncRequestErrorCounter("error namespace create completion", SEVERITY_MAJOR)
				log.Error(err, "error namespace create completion")
				return err
				// return r.appendErrorConditionAndReturn(ctx, namespace,
				// "Owning namespace failed to create within 15 seconds")
			}
			log.Info("Created Namespace: "+foundNamespace.Name, "status", foundNamespace.Status.Phase)
			justCreated = true
		} else {
			// IncRequestErrorCounter("error reading namespace", SEVERITY_MAJOR)
			log.Error(err, "Error reading namespace")
			return err
		}
	}
	if !justCreated && CopyNamespace(namespace, foundNamespace) {
		log.Info("Updating Namespace\n", "namespace", namespace.Name)
		if err := r.Update(ctx, foundNamespace); err != nil {
			log.Error(err, "Unable to update Namespace")
			return err
		}
	}
	return nil
}

// DestinationRule reconciles an Istio DestinationRule object.
func DestinationRule(ctx context.Context, r client.Client, destinationRule *istioNetworkingClient.DestinationRule, log logr.Logger) error {
	foundDestinationRule := &istioNetworkingClient.DestinationRule{}
	justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: destinationRule.Name, Namespace: destinationRule.Namespace}, foundDestinationRule); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating Istio DestinationRule", "namespace", destinationRule.Namespace, "name", destinationRule.Name)
			if err = r.Create(ctx, destinationRule); err != nil {
				log.Error(err, "Unable to create Istio DestinationRule")
				return err
			}
			justCreated = true
		} else {
			log.Error(err, "Error getting Istio DestinationRule")
			return err
		}
	}
	if !justCreated && CopyDestinationRule(destinationRule, foundDestinationRule) {
		log.Info("Updating Istio DestinationRule\n", "namespace", destinationRule.Namespace, "name", destinationRule.Name)
		if err := r.Update(ctx, foundDestinationRule); err != nil {
			log.Error(err, "Unable to update Istio DestinationRule")
			return err
		}
	}
	return nil
}

// RequestAuthentication reconciles an Istio RequestAuthentication object.
func RequestAuthentication(ctx context.Context, r client.Client, requestAuth *istioSecurityClient.RequestAuthentication, log logr.Logger) error {
	foundRequestAuth := &istioSecurityClient.RequestAuthentication{}
	justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: requestAuth.Name, Namespace: requestAuth.Namespace}, foundRequestAuth); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating Istio RequestAuthentication", "namespace", requestAuth.Namespace, "name", requestAuth.Name)
			if err = r.Create(ctx, requestAuth); err != nil {
				log.Error(err, "Unable to create Istio RequestAuthentication")
				return err
			}
			justCreated = true
		} else {
			log.Error(err, "Error getting Istio RequestAuthentication")
			return err
		}
	}
	if !justCreated && CopyRequestAuthentication(requestAuth, foundRequestAuth) {
		log.Info("Updating Istio RequestAuthentication\n", "namespace", requestAuth.Namespace, "name", requestAuth.Name)
		if err := r.Update(ctx, foundRequestAuth); err != nil {
			log.Error(err, "Unable to update Istio RequestAuthentication")
			return err
		}
	}

	return nil
}

// AuthorizationPolicy reconciles an Istio AuthorizationPolicy object.
func AuthorizationPolicy(ctx context.Context, r client.Client, authPolicy *istioSecurityClient.AuthorizationPolicy, log logr.Logger) error {
	foundAuthPolicy := &istioSecurityClient.AuthorizationPolicy{}
	justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: authPolicy.Name, Namespace: authPolicy.Namespace}, foundAuthPolicy); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating Istio AuthorizationPolicy", "namespace", authPolicy.Namespace, "name", authPolicy.Name)
			if err = r.Create(ctx, authPolicy); err != nil {
				log.Error(err, "Unable to create Istio AuthorizationPolicy")
				return err
			}
			justCreated = true
		} else {
			log.Error(err, "Error getting Istio AuthorizationPolicy")
			return err
		}
	}
	if !justCreated && CopyAuthorizationPolicy(authPolicy, foundAuthPolicy) {
		log.Info("Updating Istio AuthorizationPolicy\n", "namespace", authPolicy.Namespace, "name", authPolicy.Name)
		if err := r.Update(ctx, foundAuthPolicy); err != nil {
			log.Error(err, "Unable to update Istio AuthorizationPolicy")
			return err
		}
	}

	return nil
}

// ServiceAccount reconciles a Service Account object.
func ServiceAccount(ctx context.Context, r client.Client, serviceAccount *corev1.ServiceAccount, log logr.Logger) error {
	foundServiceAccount := &corev1.ServiceAccount{}
	justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: serviceAccount.Name, Namespace: serviceAccount.Namespace}, foundServiceAccount); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating Service Account", "namespace", serviceAccount.Namespace, "name", serviceAccount.Name)
			if err = r.Create(ctx, serviceAccount); err != nil {
				log.Error(err, "Unable to create Service Account")
				return err
			}
			justCreated = true
		} else {
			log.Error(err, "Error getting Service Account")
			return err
		}
	}
	if !justCreated && CopyServiceAccount(serviceAccount, foundServiceAccount) {
		log.Info("Updating Service Account\n", "namespace", serviceAccount.Namespace, "name", serviceAccount.Name)
		if err := r.Update(ctx, foundServiceAccount); err != nil {
			log.Error(err, "Unable to update Service Account")
			return err
		}
	}

	return nil
}

// ConfigMap reconciles a ConfigMap object.
func ConfigMap(ctx context.Context, r client.Client, configMap *corev1.ConfigMap, log logr.Logger) error {
	foundConfigMap := &corev1.ConfigMap{}
	justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating ConfigMap", "namespace", configMap.Namespace, "name", configMap.Name)
			if err = r.Create(ctx, configMap); err != nil {
				log.Error(err, "Unable to create ConfigMap")
				return err
			}
			justCreated = true
		} else {
			log.Error(err, "Error getting ConfigMap")
			return err
		}
	}
	if !justCreated && CopyConfigMap(configMap, foundConfigMap) {
		log.Info("Updating ConfigMap\n", "namespace", configMap.Namespace, "name", configMap.Name)
		if err := r.Update(ctx, foundConfigMap); err != nil {
			log.Error(err, "Unable to update ConfigMap")
			return err
		}
	}

	return nil
}

// RoleBinding reconciles a Role Binding object.
func RoleBinding(ctx context.Context, r client.Client, roleBinding *rbacv1.RoleBinding, log logr.Logger) error {
	foundRoleBinding := &rbacv1.RoleBinding{}
	justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: roleBinding.Name, Namespace: roleBinding.Namespace}, foundRoleBinding); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating Role Binding", "namespace", roleBinding.Namespace, "name", roleBinding.Name)
			if err = r.Create(ctx, roleBinding); err != nil {
				log.Error(err, "Unable to create Role Binding")
				return err
			}
			justCreated = true
		} else {
			log.Error(err, "Error getting Role Binding")
			return err
		}
	}
	if !justCreated && CopyRoleBinding(roleBinding, foundRoleBinding) {
		log.Info("Updating Role Binding\n", "namespace", roleBinding.Namespace, "name", roleBinding.Name)
		if err := r.Update(ctx, foundRoleBinding); err != nil {
			log.Error(err, "Unable to update Role Binding")
			return err
		}
	}

	return nil
}

// NetworkPolicy reconciles a NetworkPolicy object.
func NetworkPolicy(ctx context.Context, r client.Client, networkPolicy *networkv1.NetworkPolicy, log logr.Logger) error {
	foundNetworkPolicy := &networkv1.NetworkPolicy{}
	justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: networkPolicy.Name, Namespace: networkPolicy.Namespace}, foundNetworkPolicy); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating NetworkPolicy", "namespace", networkPolicy.Namespace, "name", networkPolicy.Name)
			if err = r.Create(ctx, networkPolicy); err != nil {
				log.Error(err, "Unable to create NetworkPolicy")
				return err
			}
			justCreated = true
		} else {
			log.Error(err, "Error getting NetworkPolicy")
			return err
		}
	}
	if !justCreated && CopyNetworkPolicy(networkPolicy, foundNetworkPolicy) {
		log.Info("Updating NetworkPolicy\n", "namespace", networkPolicy.Namespace, "name", networkPolicy.Name)
		if err := r.Update(ctx, foundNetworkPolicy); err != nil {
			log.Error(err, "Unable to update NetworkPolicy")
			return err
		}
	}

	return nil
}

// SubnamespaceAnchor reconciles a HNC Subnamespace object.
func SubnamespaceAnchor(ctx context.Context, r client.Client, userEnv *hncv1alpha2.SubnamespaceAnchor, log logr.Logger) error {
	foundUserEnv := &hncv1alpha2.SubnamespaceAnchor{}
	justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: userEnv.Name, Namespace: userEnv.Namespace}, foundUserEnv); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating Subnamespace", "namespace", userEnv.Namespace, "name", userEnv.Name)
			if err = r.Create(ctx, userEnv); err != nil {
				log.Error(err, "Unable to create Subnamespace")
				return err
			}
			justCreated = true
		} else {
			log.Error(err, "Error getting Istio RequestAuthentication")
			return err
		}
	}
	if !justCreated && CopySubnamespaceAnchor(userEnv, foundUserEnv) {
		log.Info("Updating Subnamespace\n", "namespace", userEnv.Namespace, "name", userEnv.Name)
		if err := r.Update(ctx, foundUserEnv); err != nil {
			log.Error(err, "Unable to update Subnamespace")
			return err
		}
	}

	return nil
}

// XPlaneIAMPolicy reconciles a CrossPlane IAM Policy object.
func XPlaneIAMPolicy(ctx context.Context, r client.Client, iamPolicy *crossplaneAWSIdentity.IAMPolicy, log logr.Logger) error {
	foundIAMPolicy := &crossplaneAWSIdentity.IAMPolicy{}
	justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: iamPolicy.Name, Namespace: iamPolicy.Namespace}, foundIAMPolicy); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating CrossPlane IAM Policy", "namespace", iamPolicy.Namespace, "name", iamPolicy.Name)
			if err = r.Create(ctx, iamPolicy); err != nil {
				log.Error(err, "Unable to create CrossPlane IAM Policy")
				return err
			}
			justCreated = true
		} else {
			log.Error(err, "Error getting CrossPlane IAM Policy")
			return err
		}
	}
	if !justCreated && CopyXPlaneIAMPolicy(iamPolicy, foundIAMPolicy) {
		log.Info("Updating CrossPlane IAM Policy\n", "namespace", iamPolicy.Namespace, "name", iamPolicy.Name)
		if err := r.Update(ctx, foundIAMPolicy); err != nil {
			log.Error(err, "Unable to update CrossPlane IAM Policy")
			return err
		}
	}

	return nil
}

// XPlaneIAMUser reconciles a CrossPlane IAM User object.
func XPlaneIAMUser(ctx context.Context, r client.Client, iamUser *crossplaneAWSIdentity.IAMUser, log logr.Logger) error {
	foundIAMUser := &crossplaneAWSIdentity.IAMUser{}
	justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: iamUser.Name, Namespace: iamUser.Namespace}, foundIAMUser); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating CrossPlane IAM User", "namespace", iamUser.Namespace, "name", iamUser.Name)
			if err = r.Create(ctx, iamUser); err != nil {
				log.Error(err, "Unable to create CrossPlane IAM User")
				return err
			}
			justCreated = true
		} else {
			log.Error(err, "Error getting CrossPlane IAM User")
			return err
		}
	}
	if !justCreated && CopyXPlaneIAMUser(iamUser, foundIAMUser) {
		log.Info("Updating CrossPlane IAM User\n", "namespace", iamUser.Namespace, "name", iamUser.Name)
		if err := r.Update(ctx, foundIAMUser); err != nil {
			log.Error(err, "Unable to update CrossPlane IAM User")
			return err
		}
	}

	return nil
}

// XPlaneIAMPolicyAttachement reconciles a CrossPlane IAM User object.
func XPlaneIAMPolicyAttachement(ctx context.Context, r client.Client, iamPolicyAttachement *crossplaneAWSIdentity.IAMUserPolicyAttachment, log logr.Logger) error {
	foundPolicyAttachement := &crossplaneAWSIdentity.IAMUserPolicyAttachment{}
	justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: iamPolicyAttachement.Name, Namespace: iamPolicyAttachement.Namespace}, foundPolicyAttachement); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating CrossPlane IAM Policy Attachement", "namespace", iamPolicyAttachement.Namespace, "name", iamPolicyAttachement.Name)
			if err = r.Create(ctx, iamPolicyAttachement); err != nil {
				log.Error(err, "Unable to create CrossPlane IAM Policy Attachement")
				return err
			}
			justCreated = true
		} else {
			log.Error(err, "Error getting CrossPlane IAM Policy Attachement")
			return err
		}
	}
	if !justCreated && CopyXPlaneIAMPolicyAttachement(iamPolicyAttachement, foundPolicyAttachement) {
		log.Info("Updating CrossPlane IAM Policy Attachement\n", "namespace", iamPolicyAttachement.Namespace, "name", iamPolicyAttachement.Name)
		if err := r.Update(ctx, foundPolicyAttachement); err != nil {
			log.Error(err, "Unable to update CrossPlane IAM Policy Attachement")
			return err
		}
	}

	return nil
}

// XPlaneIAMAccessKey reconciles a CrossPlane IAM Access Key.
func XPlaneIAMAccessKey(ctx context.Context, r client.Client, iamAccessKey *crossplaneAWSIdentity.IAMAccessKey, log logr.Logger) error {
	foundIAMAccessKey := &crossplaneAWSIdentity.IAMAccessKey{}
	justCreated := false
	if err := r.Get(ctx, types.NamespacedName{Name: iamAccessKey.Name, Namespace: iamAccessKey.Namespace}, foundIAMAccessKey); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Creating CrossPlane IAM Access Key", "namespace", iamAccessKey.Namespace, "name", iamAccessKey.Name)
			if err = r.Create(ctx, iamAccessKey); err != nil {
				log.Error(err, "Unable to create CrossPlane IAM Access Key")
				return err
			}
			justCreated = true
		} else {
			log.Error(err, "Error getting CrossPlane IAM Access Key")
			return err
		}
	}
	if !justCreated && CopyXPlaneIAMAccessKey(iamAccessKey, foundIAMAccessKey) {
		log.Info("Updating CrossPlane IAM Access Key\n", "namespace", iamAccessKey.Namespace, "name", iamAccessKey.Name)
		if err := r.Update(ctx, foundIAMAccessKey); err != nil {
			log.Error(err, "Unable to update CrossPlane IAM Access Key")
			return err
		}
	}

	return nil
}

// Reference: https://github.com/pwittrock/kubebuilder-workshop/blob/master/pkg/util/util.go

// CopyStatefulSetFields copies the owned fields from one StatefulSet to another
// Returns true if the fields copied from don't match to.
func CopyStatefulSetFields(from, to *appsv1.StatefulSet) bool {
	requireUpdate := false
	if !reflect.DeepEqual(to.Labels, from.Labels) {
		to.Labels = from.Labels
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Annotations, from.Annotations) {
		to.Annotations = from.Annotations
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Spec.Replicas, to.Spec.Replicas) {
		to.Spec.Replicas = from.Spec.Replicas
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Spec.Template.Labels, from.Spec.Template.Labels) {
		to.Spec.Template.Labels = from.Spec.Template.Labels
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Spec.Template.Annotations, from.Spec.Template.Annotations) {
		to.Spec.Template.Annotations = from.Spec.Template.Annotations
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.Volumes, from.Spec.Template.Spec.Volumes) {
		to.Spec.Template.Spec.Volumes = from.Spec.Template.Spec.Volumes
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.ServiceAccountName, from.Spec.Template.Spec.ServiceAccountName) {
		to.Spec.Template.Spec.ServiceAccountName = from.Spec.Template.Spec.ServiceAccountName
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.SecurityContext, from.Spec.Template.Spec.SecurityContext) {
		to.Spec.Template.Spec.SecurityContext = from.Spec.Template.Spec.SecurityContext
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.Containers[0].Name, from.Spec.Template.Spec.Containers[0].Name) {
		to.Spec.Template.Spec.Containers[0].Name = from.Spec.Template.Spec.Containers[0].Name
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.Containers[0].Image, from.Spec.Template.Spec.Containers[0].Image) {
		to.Spec.Template.Spec.Containers[0].Image = from.Spec.Template.Spec.Containers[0].Image
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.Containers[0].WorkingDir, from.Spec.Template.Spec.Containers[0].WorkingDir) {
		to.Spec.Template.Spec.Containers[0].WorkingDir = from.Spec.Template.Spec.Containers[0].WorkingDir
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.Containers[0].Ports, from.Spec.Template.Spec.Containers[0].Ports) {
		to.Spec.Template.Spec.Containers[0].Ports = from.Spec.Template.Spec.Containers[0].Ports
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.Containers[0].Env, from.Spec.Template.Spec.Containers[0].Env) {
		to.Spec.Template.Spec.Containers[0].Env = from.Spec.Template.Spec.Containers[0].Env
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.Containers[0].Resources, from.Spec.Template.Spec.Containers[0].Resources) {
		to.Spec.Template.Spec.Containers[0].Resources = from.Spec.Template.Spec.Containers[0].Resources
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec.Containers[0].VolumeMounts, from.Spec.Template.Spec.Containers[0].VolumeMounts) {
		to.Spec.Template.Spec.Containers[0].VolumeMounts = from.Spec.Template.Spec.Containers[0].VolumeMounts
		requireUpdate = true
	}

	return requireUpdate
}

func CopyDeploymentSetFields(from, to *appsv1.Deployment) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Labels) == 0 && len(from.Labels) != 0 {
		requireUpdate = true
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Annotations) == 0 && len(from.Annotations) != 0 {
		requireUpdate = true
	}
	to.Annotations = from.Annotations

	if from.Spec.Replicas != to.Spec.Replicas {
		to.Spec.Replicas = from.Spec.Replicas
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Spec.Template.Spec, from.Spec.Template.Spec) {
		requireUpdate = true
	}
	to.Spec.Template.Spec = from.Spec.Template.Spec

	return requireUpdate
}

// CopyServiceFields copies the owned fields from one Service to another
func CopyServiceFields(from, to *corev1.Service) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Labels) == 0 && len(from.Labels) != 0 {
		requireUpdate = true
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Annotations) == 0 && len(from.Annotations) != 0 {
		requireUpdate = true
	}
	to.Annotations = from.Annotations

	// Don't copy the entire Spec, because we can't overwrite the clusterIp field

	if !reflect.DeepEqual(to.Spec.Selector, from.Spec.Selector) {
		requireUpdate = true
	}
	to.Spec.Selector = from.Spec.Selector

	if !reflect.DeepEqual(to.Spec.Ports, from.Spec.Ports) {
		requireUpdate = true
	}
	to.Spec.Ports = from.Spec.Ports

	return requireUpdate
}

// Copy configuration related fields to another instance and returns true if there
// is a diff and thus needs to update.
func CopyVirtualService(from, to *istioNetworkingClient.VirtualService) bool {
	requireUpdate := false
	if !reflect.DeepEqual(to.Labels, from.Labels) {
		to.Labels = from.Labels
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Annotations, from.Annotations) {
		to.Annotations = from.Annotations
		requireUpdate = true
	}

	if !reflect.DeepEqual(to.Spec, to.Spec) {
		to.Spec = from.Spec
		requireUpdate = true
	}

	return requireUpdate
}

// CopyAuthorizationPolicy copies the owned fields from one AuthorizationPolicy to another
func CopyAuthorizationPolicy(from, to *istioSecurityClient.AuthorizationPolicy) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Labels) == 0 && len(from.Labels) != 0 {
		requireUpdate = true
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Annotations) == 0 && len(from.Annotations) != 0 {
		requireUpdate = true
	}
	to.Annotations = from.Annotations

	// Don't copy the entire Spec, because we this can lead to unnecessary reconciles

	if !reflect.DeepEqual(to.Spec.Selector, from.Spec.Selector) {
		requireUpdate = true
	}
	to.Spec.Selector = from.Spec.Selector

	if !reflect.DeepEqual(to.Spec.Action, from.Spec.Action) {
		requireUpdate = true
	}
	to.Spec.Action = from.Spec.Action

	if !reflect.DeepEqual(to.Spec.Rules, from.Spec.Rules) {
		requireUpdate = true
	}
	to.Spec.Rules = from.Spec.Rules

	return requireUpdate
}

// CopyRequestAuthentication copies the owned fields from one RequestAuthentication to another
func CopyRequestAuthentication(from, to *istioSecurityClient.RequestAuthentication) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Labels) == 0 && len(from.Labels) != 0 {
		requireUpdate = true
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Annotations) == 0 && len(from.Annotations) != 0 {
		requireUpdate = true
	}
	to.Annotations = from.Annotations

	// Don't copy the entire Spec, because we this can lead to unnecessary reconciles

	if !reflect.DeepEqual(to.Spec.Selector, from.Spec.Selector) {
		requireUpdate = true
	}
	to.Spec.Selector = from.Spec.Selector

	if !reflect.DeepEqual(to.Spec.JwtRules, from.Spec.JwtRules) {
		requireUpdate = true
	}
	to.Spec.JwtRules = from.Spec.JwtRules

	return requireUpdate
}

// CopyDestinationRule copies the owned fields from one DestinationRule to another
func CopyDestinationRule(from, to *istioNetworkingClient.DestinationRule) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Labels) == 0 && len(from.Labels) != 0 {
		requireUpdate = true
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Annotations) == 0 && len(from.Annotations) != 0 {
		requireUpdate = true
	}
	to.Annotations = from.Annotations

	if !reflect.DeepEqual(to.Spec, from.Spec) {
		requireUpdate = true
	}
	to.Spec = from.Spec

	return requireUpdate
}

// CopyServiceAccount copies the owned fields from one Service Account to another
func CopyServiceAccount(from, to *corev1.ServiceAccount) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Labels) == 0 && len(from.Labels) != 0 {
		requireUpdate = true
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Annotations) == 0 && len(from.Annotations) != 0 {
		requireUpdate = true
	}
	to.Annotations = from.Annotations

	// Don't copy the entire Spec, because we this will lead to unnecessary reconciles
	if !reflect.DeepEqual(to.ImagePullSecrets, from.ImagePullSecrets) {
		requireUpdate = true
	}
	to.ImagePullSecrets = from.ImagePullSecrets

	if !reflect.DeepEqual(to.AutomountServiceAccountToken, from.AutomountServiceAccountToken) {
		requireUpdate = true
	}
	to.AutomountServiceAccountToken = from.AutomountServiceAccountToken

	return requireUpdate
}

// CopyServiceAccount copies the owned fields from one Service Account to another
func CopyConfigMap(from, to *corev1.ConfigMap) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Labels) == 0 && len(from.Labels) != 0 {
		requireUpdate = true
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Annotations) == 0 && len(from.Annotations) != 0 {
		requireUpdate = true
	}
	to.Annotations = from.Annotations

	// Don't copy the entire Spec, because we this will lead to unnecessary reconciles
	if !reflect.DeepEqual(to.Data, from.Data) {
		requireUpdate = true
	}
	to.Data = from.Data

	if !reflect.DeepEqual(to.BinaryData, from.BinaryData) {
		requireUpdate = true
	}
	to.BinaryData = from.BinaryData

	return requireUpdate
}

// CopyRoleBinding copies the owned fields from one Role Binding to another
func CopyRoleBinding(from, to *rbacv1.RoleBinding) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Labels) == 0 && len(from.Labels) != 0 {
		requireUpdate = true
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Annotations) == 0 && len(from.Annotations) != 0 {
		requireUpdate = true
	}
	to.Annotations = from.Annotations

	// Don't copy the entire Spec, because we this will lead to unnecessary reconciles
	if !reflect.DeepEqual(to.RoleRef, from.RoleRef) {
		requireUpdate = true
	}
	to.RoleRef = from.RoleRef

	if !reflect.DeepEqual(to.Subjects, from.Subjects) {
		requireUpdate = true
	}
	to.Subjects = from.Subjects

	return requireUpdate
}

// CopyNetworkPolicy copies the owned fields from one NetworkPolicy to another
func CopyNetworkPolicy(from, to *networkv1.NetworkPolicy) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Labels) == 0 && len(from.Labels) != 0 {
		requireUpdate = true
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Annotations) == 0 && len(from.Annotations) != 0 {
		requireUpdate = true
	}
	to.Annotations = from.Annotations

	if !reflect.DeepEqual(to.Spec, from.Spec) {
		requireUpdate = true
	}
	to.Spec = from.Spec

	return requireUpdate
}

// CopyNamespace copies the owned fields from one Namespace to another
func CopyNamespace(from, to *corev1.Namespace) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Labels) == 0 && len(from.Labels) != 0 {
		requireUpdate = true
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Annotations) == 0 && len(from.Annotations) != 0 {
		requireUpdate = true
	}
	to.Annotations = from.Annotations

	return requireUpdate
}

// CopySubnamespaceAnchor copies the owned fields from one Subnamespace to another
func CopySubnamespaceAnchor(from, to *hncv1alpha2.SubnamespaceAnchor) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Labels) == 0 && len(from.Labels) != 0 {
		requireUpdate = true
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Annotations) == 0 && len(from.Annotations) != 0 {
		requireUpdate = true
	}
	to.Annotations = from.Annotations

	return requireUpdate
}

// CopyXPlaneIAMPolicy copies the owned fields from one CrossPlane IAM Policy to another
func CopyXPlaneIAMPolicy(from, to *crossplaneAWSIdentity.IAMPolicy) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Labels) == 0 && len(from.Labels) != 0 {
		requireUpdate = true
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Annotations) == 0 && len(from.Annotations) != 0 {
		requireUpdate = true
	}
	to.Annotations = from.Annotations

	if !reflect.DeepEqual(to.Spec, from.Spec) {
		requireUpdate = true
	}
	to.Spec = from.Spec

	return requireUpdate
}

// CopyXPlaneIAMUser copies the owned fields from one CrossPlane IAM User to another
func CopyXPlaneIAMUser(from, to *crossplaneAWSIdentity.IAMUser) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Labels) == 0 && len(from.Labels) != 0 {
		requireUpdate = true
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Annotations) == 0 && len(from.Annotations) != 0 {
		requireUpdate = true
	}
	to.Annotations = from.Annotations

	if !reflect.DeepEqual(to.Spec, from.Spec) {
		requireUpdate = true
	}
	to.Spec = from.Spec

	return requireUpdate
}

// CopyXPlaneIAMPolicyAttachement copies the owned fields from one CrossPlane IAM Policy Attachement to another
func CopyXPlaneIAMPolicyAttachement(from, to *crossplaneAWSIdentity.IAMUserPolicyAttachment) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Labels) == 0 && len(from.Labels) != 0 {
		requireUpdate = true
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Annotations) == 0 && len(from.Annotations) != 0 {
		requireUpdate = true
	}
	to.Annotations = from.Annotations

	if !reflect.DeepEqual(to.Spec, from.Spec) {
		requireUpdate = true
	}
	to.Spec = from.Spec

	return requireUpdate
}

// CopyXPlaneIAMPolicyAttachement copies the owned fields from one CrossPlane IAM Policy Attachement to another
func CopyXPlaneIAMAccessKey(from, to *crossplaneAWSIdentity.IAMAccessKey) bool {
	requireUpdate := false
	for k, v := range to.Labels {
		if from.Labels[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Labels) == 0 && len(from.Labels) != 0 {
		requireUpdate = true
	}
	to.Labels = from.Labels

	for k, v := range to.Annotations {
		if from.Annotations[k] != v {
			requireUpdate = true
		}
	}
	if len(to.Annotations) == 0 && len(from.Annotations) != 0 {
		requireUpdate = true
	}
	to.Annotations = from.Annotations

	if !reflect.DeepEqual(to.Spec.DeletionPolicy, from.Spec.DeletionPolicy) {
		requireUpdate = true
	}
	to.Spec.DeletionPolicy = from.Spec.DeletionPolicy

	if !reflect.DeepEqual(to.Spec.ForProvider, from.Spec.ForProvider) {
		requireUpdate = true
	}
	to.Spec.ForProvider = from.Spec.ForProvider

	if !reflect.DeepEqual(to.Spec.ProviderConfigReference, from.Spec.ProviderConfigReference) {
		requireUpdate = true
	}
	to.Spec.ProviderConfigReference = from.Spec.ProviderConfigReference

	return requireUpdate
}
