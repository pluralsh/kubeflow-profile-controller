/*
Copyright 2019 The Kubeflow Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/cenkalti/backoff"

	// "github.com/ghodss/yaml"

	ackIAM "github.com/aws-controllers-k8s/iam-controller/apis/v1alpha1"

	"github.com/go-logr/logr"
	"github.com/goccy/go-yaml"
	reconcilehelper "github.com/pluralsh/controller-reconcile-helper"
	profilev2alpha1 "github.com/pluralsh/kubeflow-profile-controller/apis/kubeflow.org/v2alpha1"
	platformv1alpha1 "github.com/pluralsh/kubeflow-profile-controller/apis/platform/v1alpha1"
	istioNetworking "istio.io/api/networking/v1beta1"
	istioSecurity "istio.io/api/security/v1beta1"
	"istio.io/api/type/v1beta1"
	istioNetworkingClient "istio.io/client-go/pkg/apis/networking/v1beta1"
	istioSecurityClient "istio.io/client-go/pkg/apis/security/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiResource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const AUTHZPOLICYISTIO = "ns-owner-access-istio"
const REQUESTAUTHISTIO = "kubeflow-request-auth"

// Istio constants
const ISTIOALLOWALL = "allow-all"

const KFQUOTA = "kf-resource-quota"
const PROFILEFINALIZER = "profile-finalizer"

// annotation key, consumed by kfam API
const USER = "user"
const ROLE = "role"
const ADMIN = "admin"

// Kubeflow default role names
// TODO: Make kubeflow roles configurable (krishnadurai)
// This will enable customization of roles.
const (
	kubeflowAdmin       = "kubeflow-admin"
	kubeflowEdit        = "kubeflow-edit"
	kubeflowView        = "kubeflow-view"
	istioInjectionLabel = "istio-injection"
)

const DEFAULT_EDITOR = "default-editor"
const DEFAULT_VIEWER = "default-viewer"

type Plugin interface {
	// Called when profile CR is created / updated
	ApplyPlugin(*ProfileReconciler, *profilev2alpha1.Profile) error
	// Called when profile CR is being deleted, to cleanup any non-k8s resources created via ApplyPlugin
	// RevokePlugin logic need to be IDEMPOTENT
	RevokePlugin(*ProfileReconciler, *profilev2alpha1.Profile) error
}

// ProfileReconciler reconciles a Profile object
type ProfileReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Log              logr.Logger
	WorkloadIdentity string
}

// +kubebuilder:rbac:groups=core,resources=namespaces,verbs="*"
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs="*"
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs="*"
// +kubebuilder:rbac:groups=security.istio.io,resources=authorizationpolicies,verbs="*"
// +kubebuilder:rbac:groups=iam.services.k8s.aws,resources=policies;roles,verbs="*"
// +kubebuilder:rbac:groups=platform.kubeflow.org,resources=configs,verbs="get"
// +kubebuilder:rbac:groups=kubeflow.org,resources=profiles;profiles/status;profiles/finalizers,verbs="*"

// Reconcile reads that state of the cluster for a Profile object and makes changes based on the state read
// and what is in the Profile.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
func (r *ProfileReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	// _ = log.FromContext(ctx)
	logger := r.Log.WithValues("profile", request.NamespacedName)

	// Fetch the Profile instance
	instance := &profilev2alpha1.Profile{}
	logger.Info("Start to Reconcile.", "namespace", request.Namespace, "name", request.Name)
	err := r.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			IncRequestCounter("profile deletion")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		IncRequestErrorCounter("error reading the profile object", SEVERITY_MAJOR)
		logger.Error(err, "error reading the profile object")
		return reconcile.Result{}, err
	}

	kubeflowConfig := &platformv1alpha1.Config{}
	// Get the Kubeflow config from the cluster
	if err := r.Get(ctx, types.NamespacedName{Name: "config"}, kubeflowConfig); err != nil {
		logger.Error(err, "unable to fetch Kubeflow Config")
		return ctrl.Result{}, err
	}

	// Update namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{"owner": instance.Spec.Owner.Name},
			// inject istio sidecar to all pods in target namespace by default.
			Labels: map[string]string{
				istioInjectionLabel: "enabled",
			},
			Name: instance.Name,
		},
	}
	setNamespaceLabels(ns, kubeflowConfig.Spec.Namespace.DefaultLabels)
	logger.Info("List of labels to be added to namespace", "labels", ns.Labels)
	if err := controllerutil.SetControllerReference(instance, ns, r.Scheme); err != nil {
		IncRequestErrorCounter("error setting ControllerReference", SEVERITY_MAJOR)
		logger.Error(err, "error setting ControllerReference")
		return reconcile.Result{}, err
	}
	foundNs := &corev1.Namespace{}
	err = r.Get(ctx, types.NamespacedName{Name: ns.Name}, foundNs)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Creating Namespace: " + ns.Name)
			err = r.Create(ctx, ns)
			if err != nil {
				IncRequestErrorCounter("error creating namespace", SEVERITY_MAJOR)
				logger.Error(err, "error creating namespace")
				return reconcile.Result{}, err
			}
			// wait 15 seconds for new namespace creation.
			err = backoff.Retry(
				func() error {
					return r.Get(ctx, types.NamespacedName{Name: ns.Name}, foundNs)
				},
				backoff.WithMaxRetries(backoff.NewConstantBackOff(3*time.Second), 5))
			if err != nil {
				IncRequestErrorCounter("error namespace create completion", SEVERITY_MAJOR)
				logger.Error(err, "error namespace create completion")
				return r.appendErrorConditionAndReturn(ctx, instance,
					"Owning namespace failed to create within 15 seconds")
			}
			logger.Info("Created Namespace: "+foundNs.Name, "status", foundNs.Status.Phase)
		} else {
			IncRequestErrorCounter("error reading namespace", SEVERITY_MAJOR)
			logger.Error(err, "error reading namespace")
			return reconcile.Result{}, err
		}
	} else {
		// Check exising namespace ownership before move forward
		owner, ok := foundNs.Annotations["owner"]
		if ok && owner == instance.Spec.Owner.Name {
			oldLabels := map[string]string{}
			for k, v := range foundNs.Labels {
				oldLabels[k] = v
			}
			setNamespaceLabels(foundNs, kubeflowConfig.Spec.Namespace.DefaultLabels)
			logger.Info("List of labels to be added to found namespace", "labels", ns.Labels)
			if !reflect.DeepEqual(oldLabels, foundNs.Labels) {
				err = r.Update(ctx, foundNs)
				if err != nil {
					IncRequestErrorCounter("error updating namespace label", SEVERITY_MAJOR)
					logger.Error(err, "error updating namespace label")
					return reconcile.Result{}, err
				}
			}
		} else {
			logger.Info(fmt.Sprintf("namespace already exist, but not owned by profile creator %v",
				instance.Spec.Owner.Name))
			IncRequestCounter("reject profile taking over existing namespace")
			return r.appendErrorConditionAndReturn(ctx, instance, fmt.Sprintf(
				"namespace already exist, but not owned by profile creator %v", instance.Spec.Owner.Name))
		}
	}

	// Update Istio RequestAuthentication
	// Create Istio RequestAuthentication in target namespace, which configures Istio with the OIDC provider.
	if err = r.updateIstioRequestAuthentication(instance, kubeflowConfig.Spec.Security.OIDC.Issuer, kubeflowConfig.Spec.Security.OIDC.JwksURI); err != nil {
		logger.Error(err, "error Updating Istio RequestAuthentication permission", "namespace", instance.Name)
		IncRequestErrorCounter("error updating Istio RequestAuthentication permission", SEVERITY_MAJOR)
		return reconcile.Result{}, err
	}

	// Update Istio AuthorizationPolicy
	// Create Istio AuthorizationPolicy in target namespace, which will give ns owner permission to access services in ns.
	if err = r.updateIstioAuthorizationPolicy(instance, kubeflowConfig.Spec.Identity.UserIDPrefix, kubeflowConfig.Spec.Security.OIDC.Issuer); err != nil {
		logger.Error(err, "error Updating Istio AuthorizationPolicy permission", "namespace", instance.Name)
		IncRequestErrorCounter("error updating Istio AuthorizationPolicy permission", SEVERITY_MAJOR)
		return reconcile.Result{}, err
	}

	// Update service accounts
	// Create service account "default-editor" in target namespace.
	// "default-editor" would have kubeflowEdit permission: edit all resources in target namespace except rbac.
	serviceAccounts := r.generateServiceAccounts(instance, kubeflowConfig.Spec.Infrastructure.ProviderConfig.AccountID, kubeflowConfig.Spec.Infrastructure.ClusterName)
	for _, serviceAccount := range serviceAccounts.Items {
		sa := &serviceAccount
		if err := ctrl.SetControllerReference(instance, sa, r.Scheme); err != nil {
			logger.Error(err, "Error setting ControllerReference for Service Account")
			return ctrl.Result{}, err
		}
		if err := reconcilehelper.ServiceAccount(ctx, r.Client, sa, logger); err != nil {
			logger.Error(err, "Error reconciling Service Account", "namespace", sa.Namespace, "name", sa.Name)
			IncRequestErrorCounter("error updating ServiceAccount", SEVERITY_MAJOR)
			return ctrl.Result{}, err
		}
	}

	// // Update service accounts
	// // Create service account "default-editor" in target namespace.
	// // "default-editor" would have kubeflowEdit permission: edit all resources in target namespace except rbac.
	// if err = r.updateServiceAccount(instance, DEFAULT_EDITOR, kubeflowEdit); err != nil {
	// 	logger.Error(err, "error Updating ServiceAccount", "namespace", instance.Name, "name",
	// 		"defaultEditor")
	// 	IncRequestErrorCounter("error updating ServiceAccount", SEVERITY_MAJOR)
	// 	return reconcile.Result{}, err
	// }
	// // Create service account "default-viewer" in target namespace.
	// // "default-viewer" would have k8s default "view" permission: view all resources in target namespace.
	// if err = r.updateServiceAccount(instance, DEFAULT_VIEWER, kubeflowView); err != nil {
	// 	logger.Error(err, "error Updating ServiceAccount", "namespace", instance.Name, "name",
	// 		"defaultViewer")
	// 	IncRequestErrorCounter("error updating ServiceAccount", SEVERITY_MAJOR)
	// 	return reconcile.Result{}, err
	// }

	// TODO: add role for impersonate permission

	clusterRole := kubeflowAdmin
	if instance.Spec.ClusterRole != "" {
		clusterRole = instance.Spec.ClusterRole
	}

	// Update owner rbac permission
	// When ClusterRole was referred by namespaced roleBinding, the result permission will be namespaced as well.
	roleBindings := &rbacv1.RoleBindingList{
		Items: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{USER: instance.Spec.Owner.Name, ROLE: ADMIN},
					Name:        "namespaceAdmin",
					Namespace:   instance.Name,
				},
				// Use default ClusterRole 'admin' for profile/namespace owner
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     clusterRole,
				},
				Subjects: []rbacv1.Subject{
					instance.Spec.Owner,
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      DEFAULT_EDITOR,
					Namespace: instance.Name,
				},
				// Use default ClusterRole 'admin' for profile/namespace owner
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     kubeflowEdit,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      rbacv1.ServiceAccountKind,
						Name:      DEFAULT_EDITOR,
						Namespace: instance.Name,
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      DEFAULT_VIEWER,
					Namespace: instance.Name,
				},
				// Use default ClusterRole 'admin' for profile/namespace owner
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     kubeflowView,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      rbacv1.ServiceAccountKind,
						Name:      DEFAULT_VIEWER,
						Namespace: instance.Name,
					},
				},
			},
		},
	}

	for _, roleBinding := range roleBindings.Items {
		if err = r.updateRoleBinding(instance, &roleBinding); err != nil {
			logger.Error(err, "error Updating Owner Rolebinding", "namespace", instance.Name, "name",
				instance.Spec.Owner.Name, "role", roleBinding.RoleRef.Name)
			IncRequestErrorCounter("error updating Owner Rolebinding", SEVERITY_MAJOR)
			return reconcile.Result{}, err
		}
	}
	// Create resource quota for target namespace if resources are specified in profile.
	if len(instance.Spec.ResourceQuotaSpec.Hard) > 0 {
		resourceQuota := &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      KFQUOTA,
				Namespace: instance.Name,
			},
			Spec: instance.Spec.ResourceQuotaSpec,
		}
		if err = r.updateResourceQuota(ctx, instance, resourceQuota); err != nil {
			logger.Error(err, "error Updating resource quota", "namespace", instance.Name)
			IncRequestErrorCounter("error updating resource quota", SEVERITY_MAJOR)
			return reconcile.Result{}, err
		}
	} else {
		logger.Info("No update on resource quota", "spec", instance.Spec.ResourceQuotaSpec.String())
	}
	if err := r.PatchDefaultPluginSpec(ctx, instance); err != nil {
		IncRequestErrorCounter("error patching DefaultPluginSpec", SEVERITY_MAJOR)
		logger.Error(err, "Failed patching DefaultPluginSpec", "namespace", instance.Name)
		return reconcile.Result{}, err
	}
	if plugins, err := r.GetPluginSpec(instance); err == nil {
		for _, plugin := range plugins {
			if err2 := plugin.ApplyPlugin(r, instance); err2 != nil {
				logger.Error(err2, "Failed applying plugin", "namespace", instance.Name)
				IncRequestErrorCounter("error applying plugin", SEVERITY_MAJOR)
				return reconcile.Result{}, err2
			}
		}
	}

	if err := r.reconcileExtraResources(ctx, instance); err != nil {
		IncRequestErrorCounter("error patching extra resources", SEVERITY_MAJOR)
		logger.Error(err, "Failed upserted extraResources", "namespace", instance.Name)
		return reconcile.Result{}, err
	}

	// examine DeletionTimestamp to determine if object is under deletion
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !containsString(instance.ObjectMeta.Finalizers, PROFILEFINALIZER) {
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, PROFILEFINALIZER)
			if err := r.Update(ctx, instance); err != nil {
				logger.Error(err, "error updating finalizer", "namespace", instance.Name)
				IncRequestErrorCounter("error updating finalizer", SEVERITY_MAJOR)
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if containsString(instance.ObjectMeta.Finalizers, PROFILEFINALIZER) {
			// our finalizer is present, so lets revoke all Plugins to clean up any external dependencies
			if plugins, err := r.GetPluginSpec(instance); err == nil {
				for _, plugin := range plugins {
					if err := plugin.RevokePlugin(r, instance); err != nil {
						logger.Error(err, "error revoking plugin", "namespace", instance.Name)
						IncRequestErrorCounter("error revoking plugin", SEVERITY_MAJOR)
						return reconcile.Result{}, err
					}
				}
			}

			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = removeString(instance.ObjectMeta.Finalizers, PROFILEFINALIZER)
			if err := r.Update(ctx, instance); err != nil {
				logger.Error(err, "error removing finalizer", "namespace", instance.Name)
				IncRequestErrorCounter("error removing finalizer", SEVERITY_MAJOR)
				return ctrl.Result{}, err
			}
		}
	}

	// Create the KFP configmap in the user namespace
	kfpConfigmaps := r.generateKFPConfigmap(instance, kubeflowConfig.Spec.Infrastructure.Storage.BucketName, kubeflowConfig.Spec.Infrastructure.ProviderConfig.Region)
	for _, kfpConfigmap := range kfpConfigmaps.Items {
		configmap := &kfpConfigmap
		if err := ctrl.SetControllerReference(instance, configmap, r.Scheme); err != nil {
			logger.Error(err, "Error setting ControllerReference for KFP configmap")
			return ctrl.Result{}, err
		}
		if err := reconcilehelper.ConfigMap(ctx, r.Client, configmap, logger); err != nil {
			logger.Error(err, "Error reconciling KFP Configmap", "namespace", kfpConfigmap.Namespace)
			return ctrl.Result{}, err
		}
	}

	// Create the KFP configmap in the user namespace
	kfpDeployments := r.generateKFPDeployments(instance)
	for _, kfpDeployment := range kfpDeployments.Items {
		deployment := &kfpDeployment
		if err := ctrl.SetControllerReference(instance, deployment, r.Scheme); err != nil {
			logger.Error(err, "Error setting ControllerReference for KFP Deployment")
			return ctrl.Result{}, err
		}
		if err := reconcilehelper.Deployment(ctx, r.Client, deployment, logger); err != nil {
			logger.Error(err, "Error reconciling KFP Deployment", "namespace", kfpDeployment.Namespace)
			return ctrl.Result{}, err
		}
	}

	// Create the KFP destination rules in the user namespace
	kfpDestinationRules := r.generateKFPDestinationRules(instance)
	for _, destinationRule := range kfpDestinationRules.Items {
		rule := &destinationRule
		if err := ctrl.SetControllerReference(instance, rule, r.Scheme); err != nil {
			logger.Error(err, "Error setting ControllerReference for KFP DestinationRule")
			return ctrl.Result{}, err
		}
		if err := reconcilehelper.DestinationRule(ctx, r.Client, rule, logger); err != nil {
			logger.Error(err, "Error reconciling KFP destination rule", "namespace", rule.Namespace)
			return ctrl.Result{}, err
		}
	}

	// Create the KFP AuthorizationPolicies in the user namespace
	kfpAuthorizationPolicies := r.generateKFPAuthorizationPolicies(instance)
	for _, authorizationPolicy := range kfpAuthorizationPolicies.Items {
		authPolicy := &authorizationPolicy
		if err := ctrl.SetControllerReference(instance, authPolicy, r.Scheme); err != nil {
			logger.Error(err, "Error setting ControllerReference for KFP AuthorizationPolicy")
			return ctrl.Result{}, err
		}
		if err := reconcilehelper.AuthorizationPolicy(ctx, r.Client, authPolicy, logger); err != nil {
			logger.Error(err, "Error reconciling KFP AuthorizationPolicy", "namespace", authorizationPolicy.Namespace)
			return ctrl.Result{}, err
		}
	}

	// Create the KFP Services in the user namespace
	kfpServices := r.generateKFPServices(instance)
	for _, kfpService := range kfpServices.Items {
		service := &kfpService
		if err := ctrl.SetControllerReference(instance, service, r.Scheme); err != nil {
			logger.Error(err, "Error setting ControllerReference for KFP Service")
			return ctrl.Result{}, err
		}
		if err := reconcilehelper.Service(ctx, r.Client, service, logger); err != nil {
			logger.Error(err, "Error reconciling KFP Service", "namespace", kfpService.Namespace)
			return ctrl.Result{}, err
		}
	}

	// Create the KFP IAM Policy for the user namespace
	kfpIAMPolicy := r.generateKFPIAMPolicyACK(instance, kubeflowConfig.Spec.Infrastructure.ProviderConfig.AccountID, kubeflowConfig.Spec.Infrastructure.ClusterName, kubeflowConfig.Spec.Infrastructure.Storage.BucketName)
	if err := ctrl.SetControllerReference(instance, kfpIAMPolicy, r.Scheme); err != nil {
		logger.Error(err, "Error setting ControllerReference for KFP IAM Policy")
		return ctrl.Result{}, err
	}
	if err := reconcilehelper.ACKIAMPolicy(ctx, r.Client, kfpIAMPolicy, logger); err != nil {
		logger.Error(err, "Error reconciling KFP IAM Policy", "namespace", kfpIAMPolicy.Namespace)
		return ctrl.Result{}, err
	}

	// Create the KFP IAM Role for the user namespace service account
	kfpIAMRole := r.generateKFPIAMRoleACK(instance, kubeflowConfig.Spec.Infrastructure.ProviderConfig.AccountID, strings.Replace(kubeflowConfig.Spec.Infrastructure.ProviderConfig.ClusterOIDCIssuer, "https://", "", -1), kubeflowConfig.Spec.Infrastructure.ClusterName)
	if err := ctrl.SetControllerReference(instance, kfpIAMRole, r.Scheme); err != nil {
		logger.Error(err, "Error setting ControllerReference for KFP IAM Role")
		return ctrl.Result{}, err
	}
	if err := reconcilehelper.ACKIAMRole(ctx, r.Client, kfpIAMRole, logger); err != nil {
		logger.Error(err, "Error reconciling KFP IAM Role", "namespace", kfpIAMRole.Namespace)
		return ctrl.Result{}, err
	}

	IncRequestCounter("reconcile")
	return ctrl.Result{}, nil
}

// appendErrorConditionAndReturn append failure status to profile CR and mark Reconcile done. If update condition failed, request will be requeued.
func (r *ProfileReconciler) appendErrorConditionAndReturn(ctx context.Context, instance *profilev2alpha1.Profile,
	message string) (ctrl.Result, error) {
	instance.Status.Conditions = append(instance.Status.Conditions, profilev2alpha1.ProfileCondition{
		Type:    profilev2alpha1.ProfileFailed,
		Message: message,
	})
	if err := r.Update(ctx, instance); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ProfileReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Watch config file with namespace labels. If the file changes, trigger
	// a reconciliation for all Profiles.
	profileMapperFn := handler.EnqueueRequestsFromMapFunc(func(a client.Object) []reconcile.Request {
		req := []reconcile.Request{}
		profileList := &profilev2alpha1.ProfileList{}
		err := r.List(context.TODO(), profileList)
		if err != nil {
			r.Log.Error(err, "Failed to list profiles in order to trigger reconciliation")
			return req
		}
		for _, p := range profileList.Items {
			req = append(req, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      p.Name,
					Namespace: p.Namespace,
				}})
		}
		return req
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&profilev2alpha1.Profile{}).
		Owns(&corev1.Namespace{}).
		Owns(&istioSecurityClient.AuthorizationPolicy{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&corev1.ConfigMap{}).
		// Owns(&appsv1.Deployment{}).
		Owns(&istioNetworkingClient.DestinationRule{}).
		Owns(&corev1.Service{}).
		Owns(&ackIAM.Policy{}).
		Owns(&ackIAM.Role{}).
		Watches(
			&source.Kind{Type: &platformv1alpha1.Config{}},
			profileMapperFn,
		).
		Complete(r)
}

// updateIstioRequestAuthentication create or update Istio RequestAuthentication
// resources in target namespace owned by "profileIns". The goal is to check
// the access token is valid and signed by the auth provider.
func (r *ProfileReconciler) updateIstioRequestAuthentication(profileIns *profilev2alpha1.Profile, oidcIssuer string, jwksUri string) error {
	logger := r.Log.WithValues("profile", profileIns.Name)

	istioRequestAuth := &istioSecurityClient.RequestAuthentication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      REQUESTAUTHISTIO,
			Namespace: profileIns.Name,
		},
		Spec: istioSecurity.RequestAuthentication{
			Selector: nil,
			JwtRules: []*istioSecurity.JWTRule{
				{
					Issuer:  oidcIssuer,
					JwksUri: jwksUri,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(profileIns, istioRequestAuth, r.Scheme); err != nil {
		return err
	}
	foundRequestAuthentication := &istioSecurityClient.RequestAuthentication{}
	err := r.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      istioRequestAuth.ObjectMeta.Name,
			Namespace: istioRequestAuth.ObjectMeta.Namespace,
		},
		foundRequestAuthentication,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Creating Istio RequestAuthentication", "namespace", istioRequestAuth.ObjectMeta.Namespace,
				"name", istioRequestAuth.ObjectMeta.Name)
			err = r.Create(context.TODO(), istioRequestAuth)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if !reflect.DeepEqual(istioRequestAuth, foundRequestAuthentication) {
			foundRequestAuthentication.Spec = istioRequestAuth.Spec
			logger.Info("Updating Istio RequestAuthentication", "namespace", istioRequestAuth.ObjectMeta.Namespace,
				"name", istioRequestAuth.ObjectMeta.Name, "issuer", oidcIssuer, "jwks-uri", jwksUri)
			err = r.Update(context.TODO(), foundRequestAuthentication)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ProfileReconciler) getAuthorizationPolicy(profileIns *profilev2alpha1.Profile, userIdPrefix string, oidcIssuer string) istioSecurity.AuthorizationPolicy {
	return istioSecurity.AuthorizationPolicy{
		Action: istioSecurity.AuthorizationPolicy_ALLOW,
		// Empty selector == match all workloads in namespace
		Selector: nil,
		Rules: []*istioSecurity.Rule{
			{
				From: []*istioSecurity.Rule_From{
					{
						Source: &istioSecurity.Source{
							RequestPrincipals: []string{
								fmt.Sprintf("%v/*", oidcIssuer),
							},
						},
					},
				},
				When: []*istioSecurity.Condition{
					{
						// Namespace Owner can access all workloads in the
						// namespace
						Key: fmt.Sprint("request.auth.claims[email]"), // fmt.Sprintf("request.headers[%v]", r.UserIdHeader),
						Values: []string{
							userIdPrefix + profileIns.Spec.Owner.Name,
						},
					},
				},
			},
			{
				When: []*istioSecurity.Condition{
					{
						// Workloads in the same namespace can access all other
						// workloads in the namespace
						Key:    fmt.Sprintf("source.namespace"),
						Values: []string{profileIns.Name},
					},
				},
			},
			{
				// Allow access from Kubeflow namespace for notebook culling
				From: []*istioSecurity.Rule_From{
					{
						Source: &istioSecurity.Source{
							Namespaces: []string{
								"kubeflow",
							},
						},
					},
				},
			},
			{
				To: []*istioSecurity.Rule_To{
					{
						Operation: &istioSecurity.Operation{
							// Workloads pathes should be accessible for KNative's
							// `activator` and `controller` probes
							// See: https://knative.dev/docs/serving/istio-authorization/#allowing-access-from-system-pods-by-paths
							Paths: []string{
								"/healthz",
								"/metrics",
								"/ready",
								"/wait-for-drain",
								"/v1/models/*",
							},
						},
					},
				},
				When: []*istioSecurity.Condition{
					{
						// Allow access to above paths from the knative namespace
						Key:    fmt.Sprintf("source.namespace"),
						Values: []string{"knative"},
					},
				},
			},
		},
	}
}

// updateIstioAuthorizationPolicy create or update Istio AuthorizationPolicy
// resources in target namespace owned by "profileIns". The goal is to allow
// service access for profile owner.
func (r *ProfileReconciler) updateIstioAuthorizationPolicy(profileIns *profilev2alpha1.Profile, userIdPrefix string, oidcIssuer string) error {
	logger := r.Log.WithValues("profile", profileIns.Name)

	istioAuth := &istioSecurityClient.AuthorizationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{USER: profileIns.Spec.Owner.Name, ROLE: ADMIN},
			Name:        AUTHZPOLICYISTIO,
			Namespace:   profileIns.Name,
		},
		Spec: r.getAuthorizationPolicy(profileIns, userIdPrefix, oidcIssuer),
	}

	if err := controllerutil.SetControllerReference(profileIns, istioAuth, r.Scheme); err != nil {
		return err
	}
	foundAuthorizationPolicy := &istioSecurityClient.AuthorizationPolicy{}
	err := r.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      istioAuth.ObjectMeta.Name,
			Namespace: istioAuth.ObjectMeta.Namespace,
		},
		foundAuthorizationPolicy,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Creating Istio AuthorizationPolicy", "namespace", istioAuth.ObjectMeta.Namespace,
				"name", istioAuth.ObjectMeta.Name)
			err = r.Create(context.TODO(), istioAuth)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if !reflect.DeepEqual(istioAuth, foundAuthorizationPolicy) {
			foundAuthorizationPolicy.Spec = istioAuth.Spec
			logger.Info("Updating Istio AuthorizationPolicy", "namespace", istioAuth.ObjectMeta.Namespace,
				"name", istioAuth.ObjectMeta.Name)
			err = r.Update(context.TODO(), foundAuthorizationPolicy)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// updateResourceQuota create or update ResourceQuota for target namespace
func (r *ProfileReconciler) updateResourceQuota(ctx context.Context, profileIns *profilev2alpha1.Profile,
	resourceQuota *corev1.ResourceQuota) error {
	// ctx := context.Background()
	logger := r.Log.WithValues("profile", profileIns.Name)
	if err := controllerutil.SetControllerReference(profileIns, resourceQuota, r.Scheme); err != nil {
		return err
	}
	found := &corev1.ResourceQuota{}
	err := r.Get(ctx, types.NamespacedName{Name: resourceQuota.Name, Namespace: resourceQuota.Namespace}, found)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Creating ResourceQuota", "namespace", resourceQuota.Namespace, "name", resourceQuota.Name)
			err = r.Create(ctx, resourceQuota)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if !(reflect.DeepEqual(resourceQuota.Spec, found.Spec)) {
			found.Spec = resourceQuota.Spec
			logger.Info("Updating ResourceQuota", "namespace", resourceQuota.Namespace, "name", resourceQuota.Name)
			err = r.Update(ctx, found)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ProfileReconciler) generateServiceAccounts(profileIns *profilev2alpha1.Profile, awsAccountID string, clusterName string) *corev1.ServiceAccountList {

	serviceAccounts := &corev1.ServiceAccountList{
		Items: []corev1.ServiceAccount{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      DEFAULT_EDITOR,
					Namespace: profileIns.Name,
					Annotations: map[string]string{
						"eks.amazonaws.com/role-arn": fmt.Sprintf("arn:aws:iam::%s:role/%s-kubeflow-assumable-role-ns-%s", awsAccountID, clusterName, profileIns.Name),
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      DEFAULT_VIEWER,
					Namespace: profileIns.Name,
				},
			},
		},
	}
	return serviceAccounts
}

// updateServiceAccount create or update service account "saName" with role "ClusterRoleName" in target namespace owned by "profileIns"
func (r *ProfileReconciler) updateServiceAccount(profileIns *profilev2alpha1.Profile, saName string,
	ClusterRoleName string, awsAccountID string) error {
	logger := r.Log.WithValues("profile", profileIns.Name)
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: profileIns.Name,
			Annotations: map[string]string{
				"eks.amazonaws.com/role-arn": fmt.Sprintf("arn:aws:iam::%s:role/kubeflow-assumable-role-ns-%s", awsAccountID, profileIns.Name),
			},
		},
	}
	if err := controllerutil.SetControllerReference(profileIns, serviceAccount, r.Scheme); err != nil {
		return err
	}
	found := &corev1.ServiceAccount{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: serviceAccount.Name, Namespace: serviceAccount.Namespace}, found)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Creating ServiceAccount", "namespace", serviceAccount.Namespace,
				"name", serviceAccount.Name)
			err = r.Create(context.TODO(), serviceAccount)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: profileIns.Name,
		},
		// Use default ClusterRole 'admin' for profile/namespace owner
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     ClusterRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      saName,
				Namespace: profileIns.Name,
			},
		},
	}
	return r.updateRoleBinding(profileIns, roleBinding)
}

// updateRoleBinding create or update roleBinding "roleBinding" in target namespace owned by "profileIns"
func (r *ProfileReconciler) updateRoleBinding(profileIns *profilev2alpha1.Profile,
	roleBinding *rbacv1.RoleBinding) error {
	logger := r.Log.WithValues("profile", profileIns.Name)
	if err := controllerutil.SetControllerReference(profileIns, roleBinding, r.Scheme); err != nil {
		return err
	}
	found := &rbacv1.RoleBinding{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: roleBinding.Name, Namespace: roleBinding.Namespace}, found)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Creating RoleBinding", "namespace", roleBinding.Namespace, "name", roleBinding.Name, "role", roleBinding.RoleRef.Name)
			err = r.Create(context.TODO(), roleBinding)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if !(reflect.DeepEqual(roleBinding.RoleRef, found.RoleRef) && reflect.DeepEqual(roleBinding.Subjects, found.Subjects)) {
			found.RoleRef = roleBinding.RoleRef
			found.Subjects = roleBinding.Subjects
			logger.Info("Updating RoleBinding", "namespace", roleBinding.Namespace, "name", roleBinding.Name, "role", roleBinding.RoleRef.Name)
			err = r.Update(context.TODO(), found)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// GetPluginSpec will try to unmarshal the plugin spec inside profile for the specified plugin
// Returns an error if the plugin isn't defined or if there is a problem
func (r *ProfileReconciler) GetPluginSpec(profileIns *profilev2alpha1.Profile) ([]Plugin, error) {
	logger := r.Log.WithValues("profile", profileIns.Name)
	plugins := []Plugin{}
	for _, p := range profileIns.Spec.Plugins {
		var pluginIns Plugin
		switch p.Kind {
		case KIND_WORKLOAD_IDENTITY:
			pluginIns = &GcpWorkloadIdentity{}
		case KIND_AWS_IAM_FOR_SERVICE_ACCOUNT:
			pluginIns = &AwsIAMForServiceAccount{}
		default:
			logger.Info("Plugin not recgonized: ", "Kind", p.Kind)
			continue
		}

		// To deserialize it to a specific type we need to first serialize it to bytes
		// and then unserialize it.
		specBytes, err := yaml.Marshal(p.Spec)

		if err != nil {
			logger.Info("Could not marshal plugin ", p.Kind, "; error: ", err)
			return nil, err
		}

		err = yaml.Unmarshal(specBytes, pluginIns)
		if err != nil {
			logger.Info("Could not unmarshal plugin ", p.Kind, "; error: ", err)
			return nil, err
		}
		plugins = append(plugins, pluginIns)
	}
	return plugins, nil
}

// PatchDefaultPluginSpec patch default plugins to profile CR instance if user doesn't specify plugin of same kind in CR.
func (r *ProfileReconciler) PatchDefaultPluginSpec(ctx context.Context, profileIns *profilev2alpha1.Profile) error {
	// read existing plugins into map
	plugins := make(map[string]profilev2alpha1.Plugin)
	for _, p := range profileIns.Spec.Plugins {
		plugins[p.Kind] = p
	}
	// Patch default plugins if same kind doesn't exist yet.
	if r.WorkloadIdentity != "" {
		if _, ok := plugins[KIND_WORKLOAD_IDENTITY]; !ok {
			profileIns.Spec.Plugins = append(profileIns.Spec.Plugins, profilev2alpha1.Plugin{
				TypeMeta: metav1.TypeMeta{
					Kind: KIND_WORKLOAD_IDENTITY,
				},
				Spec: &runtime.RawExtension{
					Raw: []byte(fmt.Sprintf(`{"gcpServiceAccount": "%v"}`, r.WorkloadIdentity)),
				},
			})
		}
	}
	if err := r.Update(ctx, profileIns); err != nil {
		return err
	}
	return nil
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

func setNamespaceLabels(ns *corev1.Namespace, newLabels map[string]string) {
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}

	for k, v := range newLabels {
		_, ok := ns.Labels[k]
		if len(v) == 0 {
			// When there is an empty value, k should be removed.
			if ok {
				delete(ns.Labels, k)
			}
		} else {
			if !ok {
				// Add label if not exist, otherwise skipping update.
				ns.Labels[k] = v
			}
		}
	}
}

func (r *ProfileReconciler) readDefaultLabelsFromFile(path string) map[string]string {
	logger := r.Log.WithName("read-config-file").WithValues("path", path)
	dat, err := ioutil.ReadFile(path)
	if err != nil {
		logger.Error(err, "namespace labels properties file doesn't exist")
		os.Exit(1)
	}

	labels := map[string]string{}
	err = yaml.Unmarshal(dat, &labels)
	if err != nil {
		logger.Error(err, "Unable to parse default namespace labels.")
		os.Exit(1)
	}
	return labels
}

func (r *ProfileReconciler) generateKFPConfigmap(profileIns *profilev2alpha1.Profile, pipelineBucket string, awsRegion string) *corev1.ConfigMapList {

	argoArtifactRepoString := `archiveLogs: true
s3:
  bucket: %s
  keyPrefix: pipelines
  endpoint: s3.amazonaws.com
  region: %s
  insecure: false
  keyFormat: "pipelines/{{workflow.namespace}}/pipelines/{{workflow.name}}/{{pod.name}}"`

	argoArtifactRepo := fmt.Sprintf(argoArtifactRepoString, pipelineBucket, awsRegion)

	configmaps := &corev1.ConfigMapList{
		Items: []corev1.ConfigMap{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "metadata-grpc-configmap",
					Namespace: profileIns.Name,
				},
				Data: map[string]string{
					"METADATA_GRPC_SERVICE_HOST": "metadata-grpc-service.kubeflow",
					"METADATA_GRPC_SERVICE_PORT": "8080",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kfp-launcher",
					Namespace: profileIns.Name,
				},
				Data: map[string]string{
					"defaultPipelineRoot": fmt.Sprintf("s3://%s/pipelines", pipelineBucket),
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "artifact-repositories",
					Namespace:   profileIns.Name,
					Annotations: map[string]string{"workflows.argoproj.io/default-artifact-repository": "default-v1"},
				},
				Data: map[string]string{
					"default-v1": argoArtifactRepo,
				},
			},
		},
	}
	return configmaps
}

func (r *ProfileReconciler) generateKFPDeployments(profileIns *profilev2alpha1.Profile) *appsv1.DeploymentList {
	replicas := int32(1)
	deployments := &appsv1.DeploymentList{
		Items: []appsv1.Deployment{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ml-pipeline-visualizationserver",
					Namespace: profileIns.Name,
					Labels:    map[string]string{"app": "ml-pipeline-visualizationserver"},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "ml-pipeline-visualizationserver"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "ml-pipeline-visualizationserver"},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: DEFAULT_EDITOR,
							SecurityContext:    &corev1.PodSecurityContext{},
							Containers: []corev1.Container{
								{
									Name:            "ml-pipeline-visualizationserver",
									Image:           "gcr.io/ml-pipeline/visualization-server:1.7.1",
									ImagePullPolicy: corev1.PullIfNotPresent,
									Ports: []corev1.ContainerPort{
										{
											Name:          "vis-server",
											ContainerPort: 8888,
											Protocol:      corev1.ProtocolTCP,
										},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    apiResource.MustParse("50m"),
											corev1.ResourceMemory: apiResource.MustParse("200Mi"),
										},
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    apiResource.MustParse("500m"),
											corev1.ResourceMemory: apiResource.MustParse("1Gi"),
										},
									},
								},
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ml-pipeline-ui-artifact",
					Namespace: profileIns.Name,
					Labels:    map[string]string{"app": "ml-pipeline-ui-artifact"},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "ml-pipeline-ui-artifact"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "ml-pipeline-ui-artifact"},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: DEFAULT_EDITOR,
							SecurityContext:    &corev1.PodSecurityContext{},
							Containers: []corev1.Container{
								{
									Name:            "ml-pipeline-ui-artifact",
									Image:           "gcr.io/ml-pipeline/frontend:1.7.1",
									ImagePullPolicy: corev1.PullIfNotPresent,
									Ports: []corev1.ContainerPort{
										{
											Name:          "artifact-ui",
											ContainerPort: 3000,
											Protocol:      corev1.ProtocolTCP,
										},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    apiResource.MustParse("10m"),
											corev1.ResourceMemory: apiResource.MustParse("70Mi"),
										},
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    apiResource.MustParse("100m"),
											corev1.ResourceMemory: apiResource.MustParse("500Mi"),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return deployments
}

func (r *ProfileReconciler) generateKFPDestinationRules(profileIns *profilev2alpha1.Profile) *istioNetworkingClient.DestinationRuleList {

	destinationRules := &istioNetworkingClient.DestinationRuleList{
		Items: []istioNetworkingClient.DestinationRule{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ml-pipeline-visualizationserver",
					Namespace: profileIns.Name,
				},
				Spec: istioNetworking.DestinationRule{
					Host: "ml-pipeline-visualizationserver",
					TrafficPolicy: &istioNetworking.TrafficPolicy{
						Tls: &istioNetworking.ClientTLSSettings{
							Mode: istioNetworking.ClientTLSSettings_ISTIO_MUTUAL,
						},
					},
				},
			},
		},
	}
	return destinationRules
}

func (r *ProfileReconciler) generateKFPAuthorizationPolicies(profileIns *profilev2alpha1.Profile) *istioSecurityClient.AuthorizationPolicyList {

	authorizationPolicies := &istioSecurityClient.AuthorizationPolicyList{
		Items: []istioSecurityClient.AuthorizationPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ml-pipeline-visualizationserver",
					Namespace: profileIns.Name,
				},
				Spec: istioSecurity.AuthorizationPolicy{
					Selector: &v1beta1.WorkloadSelector{
						MatchLabels: map[string]string{
							"app": "ml-pipeline-visualizationserver",
						},
					},
					Rules: []*istioSecurity.Rule{
						{
							From: []*istioSecurity.Rule_From{
								{
									Source: &istioSecurity.Source{
										Principals: []string{"cluster.local/ns/kubeflow/sa/ml-pipeline"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return authorizationPolicies
}

func (r *ProfileReconciler) generateKFPServices(profileIns *profilev2alpha1.Profile) *corev1.ServiceList {

	services := &corev1.ServiceList{
		Items: []corev1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ml-pipeline-visualizationserver",
					Namespace: profileIns.Name,
					Labels: map[string]string{
						"app": "ml-pipeline-visualizationserver",
					},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						"app": "ml-pipeline-visualizationserver",
					},
					Ports: []corev1.ServicePort{
						{
							Name:     "http-visualizationserver",
							Port:     8888,
							Protocol: corev1.ProtocolTCP,
							TargetPort: intstr.IntOrString{
								IntVal: 8888,
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ml-pipeline-ui-artifact",
					Namespace: profileIns.Name,
					Labels: map[string]string{
						"app": "ml-pipeline-ui-artifact",
					},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						"app": "ml-pipeline-ui-artifact",
					},
					Ports: []corev1.ServicePort{
						{
							Name:     "http-artifact-ui",
							Port:     80,
							Protocol: corev1.ProtocolTCP,
							TargetPort: intstr.IntOrString{
								IntVal: 3000,
							},
						},
					},
				},
			},
		},
	}
	return services
}

func (r *ProfileReconciler) generateKFPIAMPolicyACK(profileIns *profilev2alpha1.Profile, awsAccountID string, clusterName string, pipelineBucket string) *ackIAM.Policy {

	documentString := `{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Sid": "AllowUserToSeeBucketListInTheConsole",
			"Action": ["s3:ListAllMyBuckets", "s3:GetBucketLocation"],
			"Effect": "Allow",
			"Resource": ["arn:aws:s3:::*"]
		},
		{
			"Sid": "AllowRootListingOfCompanyBucket",
			"Action": ["s3:ListBucket"],
			"Effect": "Allow",
			"Resource": ["arn:aws:s3:::%s"]
		},
		{
			"Sid": "AllowRootAndHomeListingOfCompanyBucket",
			"Action": ["s3:ListBucket"],
			"Effect": "Allow",
			"Resource": ["arn:aws:s3:::%s"],
			"Condition":{"StringEquals":{"s3:prefix":["","pipelines/", "pipelines/%s"],"s3:delimiter":["/"]}}
		},
		{
			"Sid": "AllowListingOfUserFolder",
			"Action": ["s3:ListBucket"],
			"Effect": "Allow",
			"Resource": ["arn:aws:s3:::%s"],
			"Condition":{"StringLike":{"s3:prefix":["pipelines/%s/*"]}}
		},
		{
			"Sid": "kubeflowNS%s",
			"Effect": "Allow",
			"Action": "s3:*",
			"Resource": [
				"arn:aws:s3:::%s/pipelines/%s/*"
			]
		}
	]
}`

	document := fmt.Sprintf(documentString, pipelineBucket, pipelineBucket, profileIns.Name, pipelineBucket, profileIns.Name, strings.Replace(profileIns.Name, "-", "", -1), pipelineBucket, profileIns.Name)

	description := "policy for namespace S3 access"

	policyName := fmt.Sprintf("%s-kubeflow-s3-iam-policy-ns-%s", clusterName, profileIns.Name)

	path := "/"

	iamPolicy := &ackIAM.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: profileIns.Name,
		},
		Spec: ackIAM.PolicySpec{
			Description:    &description,
			Name:           &policyName,
			Path:           &path,
			PolicyDocument: &document,
		},
	}
	return iamPolicy
}

func (r *ProfileReconciler) generateKFPIAMRoleACK(profileIns *profilev2alpha1.Profile, awsAccountID string, oidcIssuer string, clusterName string) *ackIAM.Role {
	assumeRolePolicyDocumentString := `{
		"Version": "2012-10-17",
		"Statement": [
		  {
			"Sid": "",
			"Effect": "Allow",
			"Principal": {
			  "Federated": "arn:aws:iam::%s:oidc-provider/%s"
			},
			"Action": "sts:AssumeRoleWithWebIdentity",
			"Condition": {
			  "StringLike": {
				"%s:sub": [
				  "system:serviceaccount:%s:default-editor"
				]
			  }
			}
		  }
		]
	  }`

	assumeRolePolicyDocument := fmt.Sprintf(assumeRolePolicyDocumentString, awsAccountID, oidcIssuer, oidcIssuer, profileIns.Name)

	sessionDuration := int64(3600)

	roleName := fmt.Sprintf("%s-kubeflow-assumable-role-ns-%s", clusterName, profileIns.Name)

	path := "/"

	tagKey := "kubeflow-namespace"

	policyArn := fmt.Sprintf("arn:aws:iam::%s:policy/%s", awsAccountID, fmt.Sprintf("%s-kubeflow-s3-iam-policy-ns-%s", clusterName, profileIns.Name))

	iamRole := &ackIAM.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: profileIns.Name,
		},
		Spec: ackIAM.RoleSpec{
			AssumeRolePolicyDocument: &assumeRolePolicyDocument,
			MaxSessionDuration:       &sessionDuration,
			Name:                     &roleName,
			Path:                     &path,
			Policies: []*string{
				&policyArn,
			},
			Tags: []*ackIAM.Tag{
				{Key: &tagKey, Value: &profileIns.Name},
			},
		},
	}

	return iamRole
}
