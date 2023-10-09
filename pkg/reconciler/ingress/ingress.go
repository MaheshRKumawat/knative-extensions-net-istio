/*
Copyright 2019 The Knative Authors

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

package ingress

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	"google.golang.org/protobuf/testing/protocmp"
	pkgnetwork "knative.dev/pkg/network"

	istiov1beta1 "istio.io/api/networking/v1beta1"
	"istio.io/client-go/pkg/apis/networking/v1beta1"
	istiolisters "knative.dev/net-istio/pkg/client/istio/listers/networking/v1beta1"
	pkgreconciler "knative.dev/pkg/reconciler"
	"knative.dev/pkg/tracker"

	istioclientset "knative.dev/net-istio/pkg/client/istio/clientset/versioned"
	kaccessor "knative.dev/net-istio/pkg/reconciler/accessor"
	coreaccessor "knative.dev/net-istio/pkg/reconciler/accessor/core"
	istioaccessor "knative.dev/net-istio/pkg/reconciler/accessor/istio"
	"knative.dev/net-istio/pkg/reconciler/ingress/config"
	"knative.dev/net-istio/pkg/reconciler/ingress/resources"
	"knative.dev/networking/pkg/apis/networking"
	"knative.dev/networking/pkg/apis/networking/v1alpha1"
	ingressreconciler "knative.dev/networking/pkg/client/injection/reconciler/networking/v1alpha1/ingress"
	knnetlisters "knative.dev/networking/pkg/client/listers/networking/v1alpha1"
	netconfig "knative.dev/networking/pkg/config"
	"knative.dev/networking/pkg/status"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"

	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
)

const (
	virtualServiceConditionReconciled = "Reconciled"
	virtualServiceNotReconciled       = "ReconcileVirtualServiceFailed"
	notReconciledReason               = "ReconcileIngressFailed"
	notReconciledMessage              = "Ingress reconciliation failed"
)

// Reconciler implements the control loop for the Ingress resources.
type Reconciler struct {
	kubeclient kubernetes.Interface

	istioClientSet        istioclientset.Interface
	virtualServiceLister  istiolisters.VirtualServiceLister
	destinationRuleLister istiolisters.DestinationRuleLister
	gatewayLister         istiolisters.GatewayLister
	secretLister          corev1listers.SecretLister
	svcLister             corev1listers.ServiceLister
	ingressLister         knnetlisters.IngressLister

	tracker tracker.Interface

	statusManager status.Manager
}

var (
	_ ingressreconciler.Interface           = (*Reconciler)(nil)
	_ ingressreconciler.Finalizer           = (*Reconciler)(nil)
	_ coreaccessor.SecretAccessor           = (*Reconciler)(nil)
	_ istioaccessor.VirtualServiceAccessor  = (*Reconciler)(nil)
	_ istioaccessor.DestinationRuleAccessor = (*Reconciler)(nil)
)

// ReconcileKind compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Ingress resource
// with the current status of the resource.
func (r *Reconciler) ReconcileKind(ctx context.Context, ingress *v1alpha1.Ingress) pkgreconciler.Event {
	logger := logging.FromContext(ctx)

	reconcileErr := r.reconcileIngress(ctx, ingress)
	if reconcileErr != nil {
		logger.Errorw("Failed to reconcile Ingress: ", zap.Error(reconcileErr))
		// prevent KIngress error on concurrency errors that will be retried
		if !apierrs.IsConflict(reconcileErr) && !isUnableToAcquireLock(reconcileErr) {
			ingress.Status.MarkIngressNotReady(notReconciledReason, notReconciledMessage)
		}
		return reconcileErr
	}
	return nil
}

func (r *Reconciler) reconcileIngress(ctx context.Context, ing *v1alpha1.Ingress) error {
	logger := logging.FromContext(ctx)

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	ing.SetDefaults(ctx)

	ing.Status.InitializeConditions()
	logger.Infof("Reconciling ingress: %#v", ing)

	gatewayNames := map[v1alpha1.IngressVisibility]sets.Set[string]{}
	gatewayNames[v1alpha1.IngressVisibilityClusterLocal] = qualifiedGatewayNamesFromContext(ctx)[v1alpha1.IngressVisibilityClusterLocal]
	gatewayNames[v1alpha1.IngressVisibilityExternalIP] = sets.New[string]()

	ingressGateways := []*v1beta1.Gateway{}

	// Special handling for domain mappings
	var dmDeleteGateways []*v1beta1.Gateway
	var dmDeleteSecrets []*corev1.Secret
	var dmModifiedGateways []*v1beta1.Gateway
	if isOwnedByDomainMapping(ing) {
		// a KIngress related to a domain mapping has exactly one secret
		originSecrets, err := resources.GetSecrets(ing, r.secretLister)
		if err != nil {
			return err
		}

		if len(originSecrets) != 1 {
			return fmt.Errorf("expected exactly one Secret, but got %d", len(originSecrets))
		}

		var originSecret *corev1.Secret
		for _, secret := range originSecrets {
			originSecret = secret
		}

		certificateHash, err := resources.CalculateCertificateHash(originSecret)
		if err != nil {
			return err
		}

		gatewayList, err := resources.FindGatewaysByCertificateHash(r.gatewayLister, certificateHash)
		if err != nil {
			return err
		}

		if len(gatewayList) > 0 && resources.IsGatewayContainingHost(gatewayList[0], ing.Name) {
			// gateway already covers host for this certificate no need to change anything
			logger.Debugf("Gateway %s/%s is already up-to-date for KIngress %s/%s", config.IstioNamespace, gatewayList[0].Name, ing.Namespace, ing.Name)
			ingressGateways = append(ingressGateways, gatewayList[0])
		} else {
			cancelFunc, err := lockCertificate(ctx, r.GetKubeClient(), certificateHash, ing)
			if cancelFunc != nil {
				defer cancelFunc()
			}
			if err != nil {
				return err
			}

			// check if the secret mirror exists
			_, err = r.secretLister.Secrets(config.IstioNamespace).Get(certificateHash)
			if err != nil {
				if !apierrs.IsNotFound(err) {
					return err
				}

				// create the secret mirror
				logger.Infof("Creating Secret %s/%s for KIngress %s/%s", config.IstioNamespace, certificateHash, ing.Namespace, ing.Name)
				dmMirrorSecret := resources.MakeMirrorSecret(originSecret, certificateHash)
				_, err = coreaccessor.ReconcileSecret(ctx, nil, dmMirrorSecret, r)
				if err != nil {
					return err
				}
			}

			// check the Gateway
			allGatewaysByHost, err := resources.FindGatewaysByHost(r.gatewayLister, ing.Name)
			if err != nil {
				return err
			}

			if len(allGatewaysByHost) == 0 {
				// search by certificateHash
				allGatewaysByCertificateHash, err := resources.FindGatewaysByCertificateHash(r.gatewayLister, certificateHash)
				if err != nil {
					return err
				}

				if len(allGatewaysByCertificateHash) == 0 {
					// create the Gateway
					dmGateway, err := resources.MakeGateway(ctx, r.svcLister, certificateHash, ing)
					if err != nil {
						return err
					}
					logger.Infof("Creating new Gateway %s/%s for KIngress %s/%s", config.IstioNamespace, dmGateway.Name, ing.Namespace, ing.Name)

					ingressGateways = append(ingressGateways, dmGateway)
				} else if len(allGatewaysByCertificateHash) == 1 {
					dmGateway := allGatewaysByCertificateHash[0]

					// Gateway exists already, ensure the KIngress is covered
					updatedGateway, updated, err := resources.EnsureGatewayCoversKIngress(dmGateway, ing)
					if err != nil {
						return err
					}
					if updated {
						logger.Infof("Updating Gateway %s/%s for KIngress %s/%s", config.IstioNamespace, dmGateway.Name, ing.Namespace, ing.Name)
						ingressGateways = append(ingressGateways, updatedGateway)
						dmGateway = updatedGateway
					} else {
						logger.Debugf("Gateway %s/%s is already up-to-date for KIngress %s/%s", config.IstioNamespace, dmGateway.Name, ing.Namespace, ing.Name)
						ingressGateways = append(ingressGateways, dmGateway)
					}
				} else {
					var gatewayNames []string
					for _, gateway := range allGatewaysByCertificateHash {
						gatewayNames = append(gatewayNames, gateway.Name)
					}

					return fmt.Errorf("found multiple Gateways (%s) for certificate hash %s", strings.Join(gatewayNames, ", "), certificateHash)
				}
			} else if len(allGatewaysByHost) == 1 {
				// Gateway already exists, verify if it is for the desired certificate
				if resources.IsGatewayForCertificate(allGatewaysByHost[0], certificateHash) {
					dmGateway := allGatewaysByHost[0]
					ingressGateways = append(ingressGateways, dmGateway)
				} else {
					// take leadership over the old certificate
					oldCertificateHash, err := resources.GetCertificateHash(allGatewaysByHost[0])
					if err != nil {
						return err
					}
					cancelFunc, err := lockCertificate(ctx, r.GetKubeClient(), oldCertificateHash, ing)
					if cancelFunc != nil {
						defer cancelFunc()
					}
					if err != nil {
						return err
					}

					canBeUpdated, err := resources.AreAllKIngressesReferencingCertificate(r.ingressLister, r.secretLister, allGatewaysByHost[0], certificateHash)
					if err != nil {
						return err
					}

					// search by certificateHash
					allGatewaysByCertificateHash, err := resources.FindGatewaysByCertificateHash(r.gatewayLister, certificateHash)
					if err != nil {
						return err
					}

					if canBeUpdated && len(allGatewaysByCertificateHash) == 0 {
						// we can modify the Gateway to be for the other certificate
						dmGateway := resources.UpdateGatewayForNewCertificate(allGatewaysByHost[0], certificateHash)
						logger.Infof("Updating Gateway %s/%s for KIngress %s/%s", config.IstioNamespace, dmGateway.Name, ing.Namespace, ing.Name)
						ingressGateways = append(ingressGateways, dmGateway)

						// and we can delete the previous Secret
						oldSecret, err := r.secretLister.Secrets(config.IstioNamespace).Get(oldCertificateHash)
						if err != nil && !apierrs.IsNotFound(err) {
							return err
						}

						dmDeleteSecrets = append(dmDeleteSecrets, oldSecret)
					} else {
						if len(allGatewaysByCertificateHash) == 0 {
							// create the Gateway
							dmGateway, err := resources.MakeGateway(ctx, r.svcLister, certificateHash, ing)
							if err != nil {
								return err
							}
							logger.Infof("Creating new Gateway %s/%s for KIngress %s/%s", config.IstioNamespace, dmGateway.Name, ing.Namespace, ing.Name)

							ingressGateways = append(ingressGateways, dmGateway)
						} else if len(allGatewaysByCertificateHash) == 1 {
							dmGateway := allGatewaysByCertificateHash[0]

							// Gateway exists already, ensure the KIngress is covered
							updatedGateway, updated, err := resources.EnsureGatewayCoversKIngress(dmGateway, ing)
							if err != nil {
								return err
							}
							if updated {
								logger.Infof("Updating Gateway %s/%s for KIngress %s/%s", config.IstioNamespace, dmGateway.Name, ing.Namespace, ing.Name)
								dmGateway = updatedGateway
							} else {
								logger.Debugf("Gateway %s/%s is already up-to-date for KIngress %s/%s", config.IstioNamespace, dmGateway.Name, ing.Namespace, ing.Name)
							}

							ingressGateways = append(ingressGateways, dmGateway)
						} else {
							var gatewayNames []string
							for _, gateway := range allGatewaysByCertificateHash {
								gatewayNames = append(gatewayNames, gateway.Name)
							}

							return fmt.Errorf("found multiple Gateways (%s) for certificate hash %s", strings.Join(gatewayNames, ", "), certificateHash)
						}

						modifiedExistingGateway, deletionRequested, err := resources.RemoveKIngressFromGateway(allGatewaysByHost[0], ing)
						if err != nil {
							return err
						}

						if deletionRequested {
							logger.Infof("Deleting Gateway %s/%s for KIngress %s/%s", config.IstioNamespace, allGatewaysByHost[0].Name, ing.Namespace, ing.Name)
							dmDeleteGateways = append(dmDeleteGateways, allGatewaysByHost[0])

							// and we can delete the previous Secret
							oldSecret, err := r.secretLister.Secrets(config.IstioNamespace).Get(oldCertificateHash)
							if err != nil && !apierrs.IsNotFound(err) {
								return err
							}

							dmDeleteSecrets = append(dmDeleteSecrets, oldSecret)
						}

						if modifiedExistingGateway != nil {
							logger.Infof("Updating Gateway %s/%s for KIngress %s/%s", config.IstioNamespace, modifiedExistingGateway.Name, ing.Namespace, ing.Name)
							dmModifiedGateways = append(dmModifiedGateways, modifiedExistingGateway)
						}
					}
				}
			} else {
				var gatewayNames []string
				for _, gateway := range allGatewaysByHost {
					gatewayNames = append(gatewayNames, gateway.Name)
				}

				return fmt.Errorf("found multiple gateways (%s) for host %s", strings.Join(gatewayNames, ", "), ing.Name)
			}
		}
	}

	if shouldReconcileTLS(ing) && !isOwnedByDomainMapping(ing) {
		originSecrets, err := resources.GetSecrets(ing, r.secretLister)
		if err != nil {
			return err
		}
		nonWildcardSecrets, wildcardSecrets, err := resources.CategorizeSecrets(originSecrets)
		if err != nil {
			return err
		}
		targetNonwildcardSecrets, err := resources.MakeSecrets(ctx, nonWildcardSecrets, ing)
		if err != nil {
			return err
		}
		targetWildcardSecrets, err := resources.MakeWildcardSecrets(ctx, wildcardSecrets)
		if err != nil {
			return err
		}
		targetSecrets := make([]*corev1.Secret, 0, len(targetNonwildcardSecrets)+len(targetWildcardSecrets))
		targetSecrets = append(targetSecrets, targetNonwildcardSecrets...)
		targetSecrets = append(targetSecrets, targetWildcardSecrets...)
		if err := r.reconcileCertSecrets(ctx, ing, targetSecrets); err != nil {
			return err
		}

		nonWildcardIngressTLS := resources.GetNonWildcardIngressTLS(ing.GetIngressTLSForVisibility(v1alpha1.IngressVisibilityExternalIP), nonWildcardSecrets)
		ingressGateways, err = resources.MakeIngressTLSGateways(ctx, ing, nonWildcardIngressTLS, nonWildcardSecrets, r.svcLister)
		if err != nil {
			return err
		}

		// For Ingress TLS referencing wildcard certificates, we reconcile a separate Gateway
		// that will be shared by other Ingresses that reference the
		// same wildcard host. We need to handle wildcard certificate specially because Istio does
		// not fully support multiple TLS Servers (or Gateways) share the same certificate.
		// https://istio.io/docs/ops/common-problems/network-issues/
		desiredWildcardGateways, err := resources.MakeWildcardTLSGateways(ctx, wildcardSecrets, r.svcLister)
		if err != nil {
			return err
		}
		if err := r.reconcileWildcardGateways(ctx, desiredWildcardGateways, ing); err != nil {
			return err
		}
		gatewayNames[v1alpha1.IngressVisibilityExternalIP].Insert(resources.GetQualifiedGatewayNames(desiredWildcardGateways)...)
	}

	if shouldReconcileHTTPServer(ing) && !isOwnedByDomainMapping(ing) {
		httpServer := resources.MakeHTTPServer(ing.Spec.HTTPOption, getPublicHosts(ing))
		if len(ingressGateways) == 0 {
			var err error
			if ingressGateways, err = resources.MakeIngressGateways(ctx, ing, []*istiov1beta1.Server{httpServer}, r.svcLister); err != nil {
				return err
			}
		} else {
			// add HTTP Server into ingressGateways.
			for i := range ingressGateways {
				ingressGateways[i].Spec.Servers = append(ingressGateways[i].Spec.Servers, httpServer)
			}
		}
	} else if !isOwnedByDomainMapping(ing) {
		// Otherwise, we fall back to the default global Gateways for HTTP behavior.
		// We need this for the backward compatibility.
		defaultGlobalHTTPGateways := qualifiedGatewayNamesFromContext(ctx)[v1alpha1.IngressVisibilityExternalIP]
		gatewayNames[v1alpha1.IngressVisibilityExternalIP].Insert(sets.List(defaultGlobalHTTPGateways)...)
	}

	if err := r.reconcileIngressGateways(ctx, ingressGateways); err != nil {
		return err
	}
	gatewayNames[v1alpha1.IngressVisibilityExternalIP].Insert(resources.GetQualifiedGatewayNames(ingressGateways)...)

	if config.FromContext(ctx).Network.SystemInternalTLSEnabled() {
		logger.Info("reconciling DestinationRules for system-internal-tls")
		if err := r.reconcileDestinationRules(ctx, ing); err != nil {
			return err
		}
	}

	vses, err := resources.MakeVirtualServices(ing, gatewayNames)
	if err != nil {
		return err
	}

	logger.Info("Creating/Updating VirtualServices")
	if err := r.reconcileVirtualServices(ctx, ing, vses); err != nil {
		ing.Status.MarkLoadBalancerFailed(virtualServiceNotReconciled, err.Error())
		return err
	}

	if isOwnedByDomainMapping(ing) {
		// in case of a secret change, the old Gateway must be deleted, or the host removed, we persist this now after the VirtualService was updated to point to the new Gateway
		for _, gateway := range dmDeleteGateways {
			logger.Infof("Deleting Gateway %s/%s for KIngress %s/%s", gateway.Namespace, gateway.Name, ing.Namespace, ing.Name)
			if err = r.GetIstioClient().NetworkingV1beta1().Gateways(gateway.Namespace).Delete(ctx, gateway.Name, metav1.DeleteOptions{}); err != nil && !apierrs.IsNotFound(err) {
				return fmt.Errorf("failed to delete Gateway %s/%s: %w", gateway.Namespace, gateway.Name, err)
			}
		}
		for _, gateway := range dmModifiedGateways {
			logger.Infof("Updating Gateway %s/%s for KIngress %s/%s", gateway.Namespace, gateway.Name, ing.Namespace, ing.Name)
			if _, err := r.GetIstioClient().NetworkingV1beta1().Gateways(gateway.Namespace).Update(ctx, gateway, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("failed to update Gateway %s/%s: %w", gateway.Namespace, gateway.Name, err)
			}
		}
		for _, secret := range dmDeleteSecrets {
			logger.Infof("Deleting Secret %s/%s for KIngress %s/%s", secret.Namespace, secret.Name, ing.Namespace, ing.Name)
			if err = r.GetKubeClient().CoreV1().Secrets(secret.Namespace).Delete(ctx, secret.Name, metav1.DeleteOptions{}); err != nil && !apierrs.IsNotFound(err) {
				return fmt.Errorf("failed to delete Secret %s/%s: %w", secret.Namespace, secret.Name, err)
			}
		}

		if isCleanupOfOldGatewaysEnabled() {
			// delete the old Gateways, determine their names by running the existing logic that would create them
			originSecrets, err := resources.GetSecrets(ing, r.secretLister)
			if err != nil {
				return err
			}
			nonWildcardSecrets, wildcardSecrets, err := resources.CategorizeSecrets(originSecrets)
			if err != nil {
				return err
			}

			nonWildcardIngressTLS := resources.GetNonWildcardIngressTLS(ing.Spec.TLS, nonWildcardSecrets)
			allGateways, err := resources.MakeIngressTLSGateways(ctx, ing, nonWildcardIngressTLS, nonWildcardSecrets, r.svcLister)
			if err != nil {
				return err
			}

			if shouldReconcileHTTPServer(ing) && len(allGateways) == 0 {
				httpServer := resources.MakeHTTPServer(ing.Spec.HTTPOption, getPublicHosts(ing))
				if allGateways, err = resources.MakeIngressGateways(ctx, ing, []*istiov1beta1.Server{httpServer}, r.svcLister); err != nil {
					return err
				}
			}

			desiredWildcardGateways, err := resources.MakeWildcardTLSGateways(ctx, wildcardSecrets, r.svcLister)
			if err != nil {
				return err
			}
			allGateways = append(allGateways, desiredWildcardGateways...)

			for _, gateway := range allGateways {
				gateway, err = r.gatewayLister.Gateways(gateway.Namespace).Get(gateway.Name)
				if err == nil {
					logger.Infof("Deleting Gateway %s/%s for KIngress %s/%s", gateway.Namespace, gateway.Name, ing.Namespace, ing.Name)
					if err = r.GetIstioClient().NetworkingV1beta1().Gateways(gateway.Namespace).Delete(ctx, gateway.Name, metav1.DeleteOptions{}); err != nil && !apierrs.IsNotFound(err) {
						return err
					}
				}
			}
		}
	}

	// Update status
	ing.Status.MarkNetworkConfigured()

	var ready bool
	if ing.IsReady() {
		// When the kingress has already been marked Ready for this generation,
		// then it must have been successfully probed.  The status manager has
		// caching built-in, which makes this exception unnecessary for the case
		// of global resyncs.  HOWEVER, that caching doesn't help at all for
		// the failover case (cold caches), and the initial sync turns into a
		// thundering herd.
		// As this is an optimization, we don't worry about the ObservedGeneration
		// skew we might see when the resource is actually in flux, we simply care
		// about the steady state.
		logger.Debug("Kingress is ready, skipping probe.")
		ready = true
	} else {
		readyStatus, err := r.statusManager.IsReady(ctx, ing)
		if err != nil {
			return fmt.Errorf("failed to probe Ingress %s/%s: %w", ing.GetNamespace(), ing.GetName(), err)
		}
		ready = readyStatus
	}

	if ready {
		publicLbs := getLBStatus(publicGatewayServiceURLFromContext(ctx))
		privateLbs := getLBStatus(privateGatewayServiceURLFromContext(ctx))
		ing.Status.MarkLoadBalancerReady(publicLbs, privateLbs)
	} else {
		ing.Status.MarkLoadBalancerNotReady()
	}

	// TODO(zhiminx): Mark Route status to indicate that Gateway is configured.
	logger.Info("Ingress successfully synced")
	return nil
}

func getPublicHosts(ing *v1alpha1.Ingress) []string {
	hosts := sets.New[string]()
	for _, rule := range ing.Spec.Rules {
		if rule.Visibility == v1alpha1.IngressVisibilityExternalIP {
			hosts.Insert(rule.Hosts...)
		}
	}
	return sets.List(hosts)
}

func (r *Reconciler) reconcileCertSecrets(ctx context.Context, ing *v1alpha1.Ingress, desiredSecrets []*corev1.Secret) error {
	for _, certSecret := range desiredSecrets {
		// We track the origin and desired secrets so that desired secrets could be synced accordingly when the origin TLS certificate
		// secret is refreshed.
		r.tracker.TrackReference(resources.SecretRef(certSecret.Namespace, certSecret.Name), ing)
		r.tracker.TrackReference(resources.ExtractOriginSecretRef(certSecret), ing)
		if _, err := coreaccessor.ReconcileSecret(ctx, nil, certSecret, r); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) reconcileWildcardGateways(ctx context.Context, gateways []*v1beta1.Gateway, ing *v1alpha1.Ingress) error {
	for _, gateway := range gateways {
		r.tracker.TrackReference(resources.GatewayRef(gateway), ing)
		if err := r.reconcileSystemGeneratedGateway(ctx, gateway); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) reconcileIngressGateways(ctx context.Context, gateways []*v1beta1.Gateway) error {
	for _, gateway := range gateways {
		if err := r.reconcileSystemGeneratedGateway(ctx, gateway); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) reconcileSystemGeneratedGateway(ctx context.Context, desired *v1beta1.Gateway) error {
	existing, err := r.gatewayLister.Gateways(desired.Namespace).Get(desired.Name)
	if apierrs.IsNotFound(err) {
		if _, err := r.istioClientSet.NetworkingV1beta1().Gateways(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !cmp.Equal(existing.Spec.DeepCopy(), desired.Spec.DeepCopy(), protocmp.Transform()) {
		if _, err := r.istioClientSet.NetworkingV1beta1().Gateways(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) reconcileVirtualServices(ctx context.Context, ing *v1alpha1.Ingress,
	desired []*v1beta1.VirtualService) error {
	// First, create all needed VirtualServices.
	kept := sets.New[string]()
	for _, d := range desired {
		if d.GetAnnotations()[networking.IngressClassAnnotationKey] != netconfig.IstioIngressClassName {
			// We do not create resources that do not have istio ingress class annotation.
			// As a result, obsoleted resources will be cleaned up.
			continue
		}
		if _, err := istioaccessor.ReconcileVirtualService(ctx, ing, d, r); err != nil {
			if kaccessor.IsNotOwned(err) {
				ing.Status.MarkResourceNotOwned("VirtualService", d.Name)
			}
			return err
		}
		kept.Insert(d.Name)
	}

	// Now, remove the extra ones.
	selectors := map[string]string{
		networking.IngressLabelKey: ing.GetName(),                            // VS created from 0.12 on
		resources.RouteLabelKey:    ing.GetLabels()[resources.RouteLabelKey], // VS created before 0.12
	}
	for k, v := range selectors {
		vses, err := r.virtualServiceLister.VirtualServices(ing.GetNamespace()).List(
			labels.SelectorFromSet(labels.Set{k: v}))
		if err != nil {
			return fmt.Errorf("failed to list VirtualServices: %w", err)
		}

		// Sort the virtual services by name to get a stable deletion order.
		sort.Slice(vses, func(i, j int) bool {
			return vses[i].Name < vses[j].Name
		})

		for _, vs := range vses {
			n, ns := vs.Name, vs.Namespace
			if kept.Has(n) {
				continue
			}
			if !metav1.IsControlledBy(vs, ing) {
				// We shouldn't remove resources not controlled by us.
				continue
			}
			if err = r.istioClientSet.NetworkingV1beta1().VirtualServices(ns).Delete(ctx, n, metav1.DeleteOptions{}); err != nil {
				return fmt.Errorf("failed to delete VirtualService: %w", err)
			}
		}
	}
	return nil
}

func (r *Reconciler) reconcileDestinationRules(ctx context.Context, ing *v1alpha1.Ingress) error {
	var drs = sets.New[string]()
	for _, rule := range ing.Spec.Rules {
		for _, path := range rule.HTTP.Paths {
			// Currently DomainMappings point to the cluster local domain on the local gateway.
			// As there is no encryption there yet (https://github.com/knative/serving/issues/13472),
			// we cannot use upstream TLS here, so we need to skip it for DomainMappings
			if path.RewriteHost != "" {
				continue
			}

			for _, split := range path.Splits {
				svc, err := r.svcLister.Services(split.ServiceNamespace).Get(split.ServiceName)
				if err != nil {
					return fmt.Errorf("failed to get service: %w", err)
				}

				http2 := false
				for _, port := range svc.Spec.Ports {
					if port.Name == "http2" || port.Name == "h2c" {
						http2 = true
					}
				}

				hostname := pkgnetwork.GetServiceHostname(split.ServiceName, split.ServiceNamespace)

				// skip duplicate entries, as we only need one DR per unique upstream k8s service
				if !drs.Has(hostname) {
					dr := resources.MakeInternalEncryptionDestinationRule(hostname, ing, http2)
					if _, err := istioaccessor.ReconcileDestinationRule(ctx, ing, dr, r); err != nil {
						return fmt.Errorf("failed to reconcile DestinationRule: %w", err)
					}
					drs.Insert(hostname)
				}
			}
		}
	}

	return nil
}

func (r *Reconciler) FinalizeKind(ctx context.Context, ing *v1alpha1.Ingress) pkgreconciler.Event {
	logger := logging.FromContext(ctx)
	istiocfg := config.FromContext(ctx).Istio
	logger.Info("Cleaning up Gateway Servers")
	for _, gws := range [][]config.Gateway{istiocfg.IngressGateways, istiocfg.LocalGateways} {
		for _, gw := range gws {
			if err := r.reconcileIngressServers(ctx, ing, gw, []*istiov1beta1.Server{}); err != nil {
				return err
			}
		}
	}

	if isOwnedByDomainMapping(ing) {
		// we need to delete the Gateways, we cannot look at the Secret to determine the certificate hash because the Secret may not anymore exist,
		// we therefore must look at the annotations
		allGateways, err := resources.FindGatewaysByKIngress(r.gatewayLister, ing)
		if err != nil {
			return err
		}

		for _, gateway := range allGateways {
			// use a nested function so that defer works for the iteration
			err = func() error {
				certificateHash, err := resources.GetCertificateHash(gateway)
				if err != nil {
					return err
				}

				cancelFunc, err := lockCertificate(ctx, r.GetKubeClient(), certificateHash, ing)
				if cancelFunc != nil {
					defer cancelFunc()
				}
				if err != nil {
					return err
				}

				modifiedGateway, deletionRequested, err := resources.RemoveKIngressFromGateway(gateway, ing)
				if err != nil {
					return err
				}

				if deletionRequested {
					logger.Debugf("Deleting Gateway %s/%s for KIngress %s/%s", gateway.Namespace, gateway.Name, ing.Namespace, ing.Name)
					if err = r.GetIstioClient().NetworkingV1beta1().Gateways(gateway.Namespace).Delete(ctx, gateway.Name, metav1.DeleteOptions{}); err != nil && !apierrs.IsNotFound(err) {
						return fmt.Errorf("failed to delete Gateway: %w", err)
					}

					gatewaysByCertificateHash, err := resources.FindGatewaysByCertificateHash(r.gatewayLister, certificateHash)
					if err != nil {
						return err
					}

					if len(gatewaysByCertificateHash) == 0 {
						logger.Debugf("Deleting Secret %s/%s for KIngress %s/%s", gateway.Namespace, certificateHash, ing.Namespace, ing.Name)
						if err = r.GetKubeClient().CoreV1().Secrets(gateway.Namespace).Delete(ctx, certificateHash, metav1.DeleteOptions{}); err != nil && !apierrs.IsNotFound(err) {
							return fmt.Errorf("failed to delete Secret: %w", err)
						}
					}
				}

				if modifiedGateway != nil {
					logger.Debugf("Updating Gateway %s/%s for KIngress %s/%s", gateway.Namespace, gateway.Name, ing.Namespace, ing.Name)
					if err := r.reconcileSystemGeneratedGateway(ctx, modifiedGateway); err != nil {
						return err
					}
				}

				return nil
			}()

			if err != nil {
				return err
			}
		}
	}

	return r.reconcileDeletion(ctx, ing)
}

func (r *Reconciler) reconcileDeletion(ctx context.Context, ing *v1alpha1.Ingress) error {
	if !shouldReconcileTLS(ing) {
		return nil
	}

	errs := []error{}
	for _, tls := range ing.GetIngressTLSForVisibility(v1alpha1.IngressVisibilityExternalIP) {
		nameNamespaces, err := resources.GetIngressGatewaySvcNameNamespaces(ctx)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, nameNamespace := range nameNamespaces {
			secrets, err := r.GetSecretLister().Secrets(nameNamespace.Namespace).List(labels.SelectorFromSet(
				resources.MakeTargetSecretLabels(tls.SecretName, tls.SecretNamespace)))
			if err != nil {
				errs = append(errs, err)
				continue
			}
			for _, secret := range secrets {
				if err := r.GetKubeClient().CoreV1().Secrets(secret.Namespace).Delete(ctx, secret.Name, metav1.DeleteOptions{}); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return errors.NewAggregate(errs)
}

func (r *Reconciler) reconcileIngressServers(ctx context.Context, ing *v1alpha1.Ingress, gw config.Gateway, desired []*istiov1beta1.Server) error {
	gateway, err := r.gatewayLister.Gateways(gw.Namespace).Get(gw.Name)
	if err != nil {
		// Unlike VirtualService, a default gateway needs to be existent.
		// It should be installed when installing Knative.
		return fmt.Errorf("failed to get Gateway: %w", err)
	}
	existing := resources.GetServers(gateway, ing)
	return r.reconcileGateway(ctx, ing, gateway, existing, desired)
}

func (r *Reconciler) reconcileGateway(ctx context.Context, ing *v1alpha1.Ingress, gateway *v1beta1.Gateway, existing []*istiov1beta1.Server, desired []*istiov1beta1.Server) error {
	if cmp.Equal(existing, desired, protocmp.Transform()) {
		return nil
	}

	deepCopy := gateway.DeepCopy()
	deepCopy = resources.UpdateGateway(deepCopy, desired, existing)
	if _, err := r.istioClientSet.NetworkingV1beta1().Gateways(deepCopy.Namespace).Update(ctx, deepCopy, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update Gateway: %w", err)
	}
	controller.GetEventRecorder(ctx).Eventf(ing, corev1.EventTypeNormal,
		"Updated", "Updated Gateway %s/%s", gateway.Namespace, gateway.Name)
	return nil
}

// GetKubeClient returns the client to access k8s resources.
func (r *Reconciler) GetKubeClient() kubernetes.Interface {
	return r.kubeclient
}

// GetSecretLister returns the lister for Secret.
func (r *Reconciler) GetSecretLister() corev1listers.SecretLister {
	return r.secretLister
}

// GetIstioClient returns the client to access Istio resources.
func (r *Reconciler) GetIstioClient() istioclientset.Interface {
	return r.istioClientSet
}

// GetVirtualServiceLister returns the lister for VirtualService.
func (r *Reconciler) GetVirtualServiceLister() istiolisters.VirtualServiceLister {
	return r.virtualServiceLister
}

func (r *Reconciler) GetDestinationRuleLister() istiolisters.DestinationRuleLister {
	return r.destinationRuleLister
}

// qualifiedGatewayNamesFromContext get gateway names from context
func qualifiedGatewayNamesFromContext(ctx context.Context) map[v1alpha1.IngressVisibility]sets.Set[string] {
	ci := config.FromContext(ctx).Istio
	publicGateways := sets.New[string]()
	for _, gw := range ci.IngressGateways {
		publicGateways.Insert(gw.QualifiedName())
	}

	privateGateways := sets.New[string]()
	for _, gw := range ci.LocalGateways {
		privateGateways.Insert(gw.QualifiedName())
	}

	return map[v1alpha1.IngressVisibility]sets.Set[string]{
		v1alpha1.IngressVisibilityExternalIP:   publicGateways,
		v1alpha1.IngressVisibilityClusterLocal: privateGateways,
	}
}

func publicGatewayServiceURLFromContext(ctx context.Context) string {
	cfg := config.FromContext(ctx).Istio
	if len(cfg.IngressGateways) > 0 {
		return cfg.IngressGateways[0].ServiceURL
	}
	return ""
}

func privateGatewayServiceURLFromContext(ctx context.Context) string {
	cfg := config.FromContext(ctx).Istio
	if len(cfg.LocalGateways) > 0 {
		return cfg.LocalGateways[0].ServiceURL
	}
	return ""
}

// getLBStatus gets the LB Status.
func getLBStatus(gatewayServiceURL string) []v1alpha1.LoadBalancerIngressStatus {
	// The Ingress isn't load-balanced by any particular
	// Service, but through a Service mesh.
	if gatewayServiceURL == "" {
		return []v1alpha1.LoadBalancerIngressStatus{
			{MeshOnly: true},
		}
	}
	return []v1alpha1.LoadBalancerIngressStatus{
		{DomainInternal: gatewayServiceURL},
	}
}

func shouldReconcileTLS(ing *v1alpha1.Ingress) bool {
	return isIngressPublic(ing) && len(ing.GetIngressTLSForVisibility(v1alpha1.IngressVisibilityExternalIP)) > 0
}

func shouldReconcileHTTPServer(ing *v1alpha1.Ingress) bool {
	// We will create a Ingress specific HTTPServer when
	// 1. auto TLS is enabled as in this case users want us to fully handle the TLS/HTTP behavior,
	// 2. HTTPOption is set to Redirected as we don't have default HTTP server supporting HTTP redirection.
	return isIngressPublic(ing) && (ing.Spec.HTTPOption == v1alpha1.HTTPOptionRedirected || len(ing.GetIngressTLSForVisibility(v1alpha1.IngressVisibilityExternalIP)) > 0)
}

func isIngressPublic(ing *v1alpha1.Ingress) bool {
	for _, rule := range ing.Spec.Rules {
		if rule.Visibility == v1alpha1.IngressVisibilityExternalIP {
			return true
		}
	}
	return false
}
